// Package promptpersona contributes who this is, as text somebody wrote.
//
// Not a constant, and not a category: a file, with a box to edit it. A persona is the
// thing people most want to change and least want to recompile, so it lives in
// ~/.lola/persona.md, is read whenever the prompt is assembled, and can be edited in
// the settings panel. Put nothing there and there is no persona part at all.
package promptpersona

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/prompt"
	"github.com/usually-broken/lola/core/routes"
)

// Order puts persona at the front: it is the first thing read.
const Order = 5

// Path is where the box loads from and saves to.
const Path = "/api/persona"

// Where the section appears in the settings panel.
const (
	Slot  = "settings"
	Title = "Persona"
)

// DefaultPath is the file itself.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".lola", "persona.md")
}

func init() {
	plugin.Register("promptpersona", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{registry: d.Prompt, panes: d.Panes, routes: d.Routes, path: DefaultPath()}, nil
	})
}

// Plugin contributes the prompt part, the settings box, and the way they talk.
type Plugin struct {
	registry *prompt.Registry
	panes    plugin.Panes
	routes   *routes.Registry
	path     string
}

func (p *Plugin) Name() string { return "promptpersona" }

func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindPrompt,
		What: "Who this is: a file at ~/.lola/persona.md, with a box in settings to edit it.",
	}
}

// Text is the persona as it stands, or empty if nobody has written one.
func (p *Plugin) Text() string {
	if p.path == "" {
		return ""
	}
	body, err := os.ReadFile(p.path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

// body is the settings section: a textarea and one button, in the panel's own markup
// so it looks like the place it appears in rather than a form bolted onto it.
const body = `  <h2>Persona</h2>
  <p class="intro">Who this is, in your own words. It is the first thing the model reads, and it takes effect on the next reply — no restart.</p>

  <div class="field">
    <div class="label">Text</div>
    <div class="why">Goes at the top of the prompt, before any rules or facts.</div>
    <textarea id="personaText" rows="10" spellcheck="false" placeholder="Nothing yet. Write who this should be."></textarea>
  </div>

  <div class="field">
    <div class="label">Saved to</div>
    <div class="why" id="personaWhere">~/.lola/persona.md</div>
    <button id="personaSave" type="button">Save persona</button>
    <span id="personaNote" class="why"></span>
  </div>`

// Setup offers the part, the section, and the routes behind the section.
func (p *Plugin) Setup() (func() error, error) {
	if p.registry != nil {
		// Live, so an edit takes effect on the next turn rather than the next
		// restart. Nothing at all when the file is missing: an empty part would be
		// a blank line in the prompt claiming to be somebody.
		if err := p.registry.Add(prompt.Part{
			Name:  "persona",
			Order: Order,
			Live:  p.Text,
			About: "Who this is. Edited in settings, or in ~/.lola/persona.md.",
		}); err != nil {
			return nil, err
		}
	}

	if p.panes != nil {
		p.panes.Add(plugin.Pane{Slot: Slot, Title: Title, Order: 15, Body: body})
	}

	if p.routes != nil {
		if err := p.routes.Handle(Path, http.HandlerFunc(p.serve)); err != nil {
			return nil, err
		}
	}

	return func() error {
		if p.registry != nil {
			p.registry.Remove("persona")
		}
		if p.panes != nil {
			p.panes.Remove(Title)
		}
		if p.routes != nil {
			p.routes.Remove(Path)
		}
		return nil
	}, nil
}

// serve reads and writes the file the persona lives in.
func (p *Plugin) serve(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"text": p.Text(),
			"path": p.path,
		})

	case http.MethodPost:
		var ask struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&ask); err != nil {
			http.Error(w, "persona: "+err.Error(), http.StatusBadRequest)
			return
		}

		if p.path == "" {
			http.Error(w, "persona: nowhere to save it", http.StatusInternalServerError)
			return
		}
		if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
			http.Error(w, "persona: "+err.Error(), http.StatusInternalServerError)
			return
		}
		text := strings.TrimSpace(ask.Text)
		if err := os.WriteFile(p.path, []byte(text+"\n"), 0o644); err != nil {
			http.Error(w, "persona: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"saved": true, "length": len(text)})

	default:
		http.Error(w, "persona: GET or POST", http.StatusMethodNotAllowed)
	}
}

var _ core.Plugin = (*Plugin)(nil)
