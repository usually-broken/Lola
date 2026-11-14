// Package speeds reads the timings stored with replies and shows them.
//
// Every reply carries how long it took to start and how fast it then arrived.
// Alone, that is one number under one answer; together they answer the question
// people actually ask — which model is quick, and has it got slower.
package speeds

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/llm"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/routes"
	coresession "github.com/usually-broken/lola/core/session"
)

const (
	// SeriesPath is one point per reply, oldest first — what a line needs.
	SeriesPath = "/api/speeds/series"

	// SessionsPath lists conversations that have measurements in them.
	SessionsPath = "/api/speeds/sessions"

	// ClientPath serves the section's script; markup alone is inert.
	ClientPath = "/api/speeds/client.js"
)

func init() {
	plugin.Register("speeds", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{panes: d.Panes, routes: d.Routes, sessions: d.Sessions}, nil
	})
}

// Plugin offers the section.
type Plugin struct {
	panes    plugin.Panes
	routes   *routes.Registry
	sessions plugin.Sessions
}

func (p *Plugin) Name() string { return "speeds" }

func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindKeeper,
		What: "How quickly each model starts and writes, over any recent stretch.",
	}
}

// Setup offers the section and the addresses behind it.
func (p *Plugin) Setup() (func() error, error) {
	if p.routes != nil {
		for path, handler := range map[string]http.HandlerFunc{
			SeriesPath:   p.series,
			SessionsPath: p.listSessions,
			ClientPath:   p.client,
		} {
			if err := p.routes.Handle(path, handler); err != nil {
				return nil, err
			}
		}
	}

	if p.panes != nil {
		p.panes.Add(plugin.Pane{
			Slot:  "settings",
			Title: "Speeds",
			Order: 40,
			Body:  sectionMarkup,
		})
	}

	return func() error {
		if p.routes != nil {
			p.routes.Remove(SeriesPath)
			p.routes.Remove(SessionsPath)
			p.routes.Remove(ClientPath)
		}
		if p.panes != nil {
			p.panes.Remove("Speeds")
		}
		return nil
	}, nil
}

// sectionMarkup is the section, in the card's own classes.
var sectionMarkup = `
<h2>Speeds</h2>
<p class="intro">How long each model takes to say its first word (TTFT), and how fast it writes after that (TPS) — from the provider's own token counts.</p>

<div class="field">
  <div class="label">Look at</div>
  <div class="why">One conversation, or everything recorded.</div>
  <select id="speedsSession"><option value="">All conversations</option></select>
</div>

<div class="field">
  <div class="label">Over</div>
  <div class="why">Which replies are included.</div>
  <select id="speedsWindow">
    <option value="10">Last 10 replies</option>
    <option value="100">Last 100 replies</option>
    <option value="24h">Last 24 hours</option>
    <option value="7d">Last 7 days</option>
    <option value="all">All time</option>
  </select>
</div>

<div id="speedsLegend" class="why"></div>
<div id="speedsCharts"></div>
<p id="speedsNote" class="why"></p>
<script src="` + ClientPath + `"></script>`

// client serves the section's script, never cached.
func (p *Plugin) client(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	_, _ = w.Write([]byte(clientJS))
}

// series gives one point per reply, oldest first.
func (p *Plugin) series(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	type point struct {
		Model    string    `json:"model"`
		At       time.Time `json:"at"`
		TTFT     int64     `json:"ttft"`
		TPS      float64   `json:"tps,omitempty"`
		Streamed bool      `json:"streamed"`
	}

	found := samples(p.sessions, q.Get("session"), q.Get("window"))
	out := make([]point, 0, len(found))
	for i := len(found) - 1; i >= 0; i-- {
		s := found[i]
		out = append(out, point{
			Model:    s.model,
			At:       s.at,
			TTFT:     s.firstMS,
			TPS:      s.rate,
			Streamed: s.streamed,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// listSessions names the conversations that have anything measured.
func (p *Plugin) listSessions(w http.ResponseWriter, r *http.Request) {
	type row struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}

	ids := map[string]bool{}
	for _, s := range samples(p.sessions, "", "all") {
		ids[s.session] = true
	}

	out := []row{}
	if p.sessions != nil {
		if list, err := p.sessions.List(); err == nil {
			for _, info := range list {
				if ids[info.ID] {
					out = append(out, row{ID: info.ID, Title: info.Title})
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// sample is one reply's measurements, flattened for averaging.
type sample struct {
	model    string
	at       time.Time
	session  string
	firstMS  int64
	rate     float64
	streamed bool
}

// samples reads every stored reply and keeps the ones the window asks for.
//
// It goes through the store the harness already has rather than reading the
// files itself, so where conversations are kept stays the store's business.
func samples(sessions plugin.Sessions, session, window string) []sample {
	if sessions == nil {
		return nil
	}
	reader, ok := sessions.(interface {
		ListFull() ([]*coresession.Session, error)
	})
	if !ok {
		return nil
	}
	saved, err := reader.ListFull()
	if err != nil {
		return nil
	}

	found := make([]sample, 0, len(saved)*8)
	for _, sess := range saved {
		if session != "" && sess.ID != session {
			continue
		}
		for _, m := range sess.Messages {
			if m.Role != llm.RoleAssistant || m.Timing == nil {
				continue
			}
			model := m.Timing.Model
			if model == "" {
				model = "unknown"
			}
			// A reply that arrived in one piece, or in far fewer pieces than it
			// has tokens, was not seen decoding: its rate is not a measurement
			// of anything, so it is not plotted.
			streamed := m.Timing.Chunks > 0 &&
				m.Timing.Tokens > 0 &&
				m.Timing.Chunks*10 >= m.Timing.Tokens

			// A burst's rate is the size of the flush, not the speed of the
			// model, so it is dropped rather than presented as a measurement.
			rate := m.Timing.TokensPerSecond
			if !streamed {
				rate = 0
			}

			found = append(found, sample{
				model:    model,
				at:       m.Timing.At,
				session:  sess.ID,
				firstMS:  m.Timing.FirstWordMS,
				rate:     rate,
				streamed: streamed,
			})
		}
	}

	sort.Slice(found, func(i, j int) bool { return found[i].at.After(found[j].at) })
	return windowed(found, window, time.Now())
}

// windowed keeps the most recent stretch the name asks for.
func windowed(all []sample, window string, now time.Time) []sample {
	switch window {
	case "all":
		return all
	case "100":
		return first(all, 100)
	case "24h":
		return since(all, now.Add(-24*time.Hour))
	case "7d":
		return since(all, now.Add(-7*24*time.Hour))
	default:
		return first(all, 10)
	}
}

func first(all []sample, n int) []sample {
	if len(all) <= n {
		return all
	}
	return all[:n]
}

func since(all []sample, cutoff time.Time) []sample {
	for i, s := range all {
		if s.at.Before(cutoff) {
			return all[:i]
		}
	}
	return all
}

// line is the one-line form of a measurement, used by panes built from rows.
func line(firstMS int64, rate float64) string {
	return strconv.FormatInt(firstMS, 10) + "ms • " + strconv.Itoa(int(rate)) + "/s"
}

var _ core.Plugin = (*Plugin)(nil)
