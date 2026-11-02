// Package sessionlist serves the list of conversations.
//
// The page drew this from its own handler, which meant the frontend held the
// code that knew what a saved conversation looks like. This owns the list
// instead, and reads it through the seam on Deps — so it knows what a
// conversation is, not where they are kept.
package sessionlist

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/routes"
)

func init() {
	plugin.Register("sessionlist", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{routes: d.Routes, sessions: d.Sessions}, nil
	})
}

// Path is where the list is served. It is the same address the page already
// asks, so the frontend did not have to change to stop owning it.
const Path = "/api/sessions"

// Plugin answers at Path with one row per saved conversation.
type Plugin struct {
	routes   *routes.Registry
	sessions plugin.Sessions
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "sessionlist" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindUI,
		What: "Serves the list of conversations the sidebar shows.",
	}
}

// Setup offers the path, or does nothing when nothing can answer HTTP.
func (p *Plugin) Setup() (func() error, error) {
	if p.routes == nil {
		return func() error { return nil }, nil
	}
	if err := p.routes.Handle(Path, p.list); err != nil {
		return nil, err
	}
	return func() error {
		p.routes.Remove(Path)
		return nil
	}, nil
}

// row is one conversation as the list draws it: enough to show, and nothing
// about how it is stored.
type row struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Updated string `json:"updated"`
	Pinned  bool   `json:"pinned"`
	Count   int    `json:"count"`
}

func (p *Plugin) list(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "sessions: GET only", http.StatusMethodNotAllowed)
		return
	}
	if p.sessions == nil {
		http.Error(w, "sessions: nothing here keeps conversations", http.StatusServiceUnavailable)
		return
	}

	saved, err := p.sessions.List()
	if err != nil {
		http.Error(w, "sessions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rows := make([]row, 0, len(saved))
	for _, s := range saved {
		rows = append(rows, row{
			ID:      s.ID,
			Title:   s.Title,
			Updated: s.Updated.Format(time.RFC3339),
			Pinned:  s.Pinned,
			Count:   s.Count,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rows)
}

// Assert at compile time that this plugin satisfies the seam every plugin does.
var _ core.Plugin = (*Plugin)(nil)
