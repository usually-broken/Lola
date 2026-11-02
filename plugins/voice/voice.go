// Package voice offers voice mode: the control that turns it on, and the line the
// model is told while it is on.
//
// Two halves of the same thing, deliberately in one plugin: the mode exists only while
// the control says it does, and the prompt part is empty the rest of the time — so a
// text conversation is exactly what it was before voice existed.
package voice

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/prompt"
	"github.com/usually-broken/lola/core/routes"
)

const (
	// PlaceAboveComposer is the row of chips above the message box.
	PlaceAboveComposer = "above-composer"

	// Name is how the page recognises the control it opens the orb from.
	Name = "voice"

	// Path is where the page says whether voice mode is on.
	Path = "/api/voice/mode"

	// TracePath takes what the page is measuring and writes it to
	// ~/.lola/voice-trace.log. Numbers changing on screen faster than anyone can read
	// are not evidence; a file is.
	TracePath = "/api/voice/trace"
)

// instruction is what the model is told while voice is on. It says what is actually
// different about voice: the words arrived through a recogniser, and the person is
// listening rather than reading, so there is nothing to scroll back to.
const instruction = `You are being spoken to, and your reply will be read aloud. Answer in one to three sentences and stop. Never more than four unless asked for detail — there is no screen to skim, so a long reply is a long wait.

The words you receive came from a speech recogniser, so expect small errors: a name may be mangled, a number may arrive wrong, and the sentence may stop awkwardly. If a request does not make sense, say briefly what you think you heard and ask — do not guess at something destructive.

Before anything that changes files, runs a command, or costs money, say plainly what you are about to do. The person cannot see a screen, so the sentence before the call is the only warning they get.
`

// label is what a frontend that cannot draw the icon falls back to.
const label = "Voice"

// Icon is offered as markup, because a waveform says "voice" faster than a word does
// and the page should not have to keep a picture of one.
const Icon = `<svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 12h1.5M7 8v8M11 5v14M15 9v6M19 11v2M21 12h0"/></svg>`

func init() {
	plugin.Register("voice", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{out: d.Contributions, registry: d.Prompt, routes: d.Routes, panes: d.Panes, sessions: d.Sessions}, nil
	})
}

// Plugin offers the control and the prompt part.
type Plugin struct {
	out      plugin.Contributions
	registry *prompt.Registry

	// sessions is where the record keeps what happened to a reply afterwards — being
	// spoken and then cut off is part of the conversation, not a detail of this turn.
	sessions plugin.Sessions

	// cut is the sentence an interruption landed in, waiting to be explained to the
	// model on its next turn. Single use: see takeCut.
	cut    cut
	cuts   []string // recent interruptions, kept so history explains a stopped answer
	routes *routes.Registry
	panes  plugin.Panes

	mu sync.Mutex
	on bool
}

func (p *Plugin) Name() string { return "voice" }

func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindVoice,
		What: "Voice mode: the control, the ball, and what the model is told while it is listening.",
	}
}

// Setup offers the chip, the prompt part, and the route the page flips.
func (p *Plugin) Setup() (func() error, error) {
	if p.out != nil {
		// A fragment, because it is a picture: markup from a plugin compiled into the
		// harness, so it is the harness's own code rather than a stranger's.
		p.out.Add(plugin.Contribution{
			Place: PlaceAboveComposer,
			Kind:  "fragment",
			Name:  Name,
			Label: label,
			Value: `<button class="chip voice-open" data-voice-toggle type="button" title="Voice">` + Icon + `</button>`,
		})
	}

	// The ball, in its own place, with its behaviour served from this plugin.
	if p.out != nil {
		p.out.Add(plugin.Contribution{
			Place: PlaceVoiceMode,
			Kind:  "fragment",
			Name:  "orb",
			Value: orbMarkup,
		})
	}

	if p.registry != nil {
		if err := p.registry.Add(prompt.Part{
			Name:  "voice",
			Order: 25,
			Live:  p.text,
			About: "What being spoken to changes: expect recognition errors, warn before acting, no skimming.",
		}); err != nil {
			return nil, err
		}
	}

	if p.routes != nil {
		handles := []struct {
			at      string
			handler http.HandlerFunc
		}{
			{Path, p.serve},
			{InterruptPath, p.HandleInterrupt},
			{TracePath, p.trace},
			{ScriptPath, p.script},
			{SettingsPathRoute, p.settingsHandler},
			{SettingsScript, p.serveSettingsScript},
		}

		done := make([]string, 0, len(handles))
		for _, h := range handles {
			if err := p.routes.Handle(h.at, h.handler); err != nil {
				// Anything offered so far goes back: a half-registered plugin is a
				// plugin that answers some of its addresses and not others.
				for _, at := range done {
					p.routes.Remove(at)
				}
				return nil, err
			}
			done = append(done, h.at)
		}
	}

	// The settings section, with its own markup and its own behaviour.
	if p.panes != nil {
		p.panes.Add(plugin.Pane{Slot: "settings", Title: "Voice", Order: 20, Body: settingsSection})
	}

	return func() error {
		// The contribution registry has no Remove yet: the page only draws what a
		// mounted plugin offered, so unmounting is enough to make the chip go away.
		if p.registry != nil {
			p.registry.Remove("voice")
		}
		if p.panes != nil {
			p.panes.Remove("Voice")
		}
		if p.routes != nil {
			for _, at := range []string{Path, TracePath, ScriptPath, SettingsPathRoute, SettingsScript} {
				p.routes.Remove(at)
			}
		}
		return nil
	}, nil
}

// text is the prompt part, and nothing at all when voice is off.
func (p *Plugin) text() string {
	p.mu.Lock()
	on := p.on
	p.mu.Unlock()

	// History is not conditional on the microphone being open. An answer that stopped
	// mid-thought stays stopped, and the next turn needs to know that whether or not
	// voice mode happens to be on when it is written.
	p.mu.Lock()
	recent := append([]string(nil), p.cuts...)
	p.mu.Unlock()

	if !on {
		return cutHistory(recent)
	}
	// The cut is taken outside the lock, because taking it locks: holding one lock and
	// asking for the same one is how a prompt assembly deadlocks the whole harness.
	c := p.takeCut()
	note := cutNote(c)
	if note != "" {
		// The note is spent as it is read, so the only record of it ever having been said
		// is this line. Without it, "did it land?" is unanswerable after the fact.
		traceLine("cut: explained, ending at \"" + c.Sentence + "\"")
	}
	return instruction + cutHistory(recent) + note
}

// traceLine appends one line to the trace file. Best effort: a trace that cannot be
// written must never break the thing it is tracing.
func traceLine(line string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(home, ".lola", "voice-trace.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05.000"), line)
}

// Enabled reports whether voice mode is on.
func (p *Plugin) Enabled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.on
}

// serve flips the mode, and says which way it went.
func (p *Plugin) serve(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"on": p.Enabled()})

	case http.MethodPost:
		var ask struct {
			On *bool `json:"on"`
		}
		if err := json.NewDecoder(r.Body).Decode(&ask); err != nil {
			http.Error(w, "voice: "+err.Error(), http.StatusBadRequest)
			return
		}

		p.mu.Lock()
		if ask.On == nil {
			p.on = !p.on
		} else {
			p.on = *ask.On
		}
		current := p.on
		p.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"on": current})

	default:
		http.Error(w, "voice: GET or POST", http.StatusMethodNotAllowed)
	}
}

var _ core.Plugin = (*Plugin)(nil)

// trace appends a line about what the page is measuring. It is deliberately dumb:
// a log that outlives the tab is the only way to find out why nothing happened.
func (p *Plugin) trace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "voice: POST only", http.StatusMethodNotAllowed)
		return
	}

	body := make([]byte, 2048)
	n, _ := r.Body.Read(body)

	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, "voice: "+err.Error(), http.StatusInternalServerError)
		return
	}
	path := filepath.Join(home, ".lola", "voice-trace.log")

	line := time.Now().Format("15:04:05.000") + " " + string(body[:n]) + "\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		http.Error(w, "voice: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	f.WriteString(line)

	w.WriteHeader(http.StatusNoContent)
}
