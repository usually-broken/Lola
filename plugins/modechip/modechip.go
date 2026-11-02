// Package modechip offers the mode selector to whatever interface is running.
//
// It exists to show the shape of things: a plugin that changes the interface
// rather than the agent, using two seams — the modes it offers a choice of, and
// the contributions where it puts that choice. Everything about how that choice
// looks is the frontend's business, which is why this file says "selector" and
// never says "button".
package modechip

import (
	"encoding/json"
	"net/http"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/routes"
)

// PlaceAboveComposer is the row of chips above the message box. The name is the
// frontend's; a frontend that does not know it simply does not draw it there.
const PlaceAboveComposer = "above-composer"

func init() {
	plugin.Register("modechip", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{modes: d.Modes, out: d.Contributions, tree: d.Plugins, routes: d.Routes}, nil
	})
}

// Plugin contributes the mode selector.
type Plugin struct {
	modes  plugin.Modes
	out    plugin.Contributions
	routes *routes.Registry

	// tree is asked, when this plugin has something to say, whether anything is
	// listening. Nil is normal: a harness with no interface is still a harness.
	tree plugin.Plugins
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "modechip" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindChip,
		What: "The control for how much the harness asks before it acts.",
	}
}

const (
	// ListPath says what modes there are, and which one is in force.
	ListPath = "/api/modes"

	// SetPath changes it, for the next call rather than the next start.
	SetPath = "/api/modes/set"
)

// Setup offers the selector and the two addresses behind it. If nothing is
// offering modes, or nothing is collecting contributions, it does nothing at all
// rather than putting a control on screen that could not work.
func (p *Plugin) Setup() (func() error, error) {
	if p.routes != nil && p.modes != nil {
		if err := p.routes.Handle(ListPath, p.list); err != nil {
			return nil, err
		}
		if err := p.routes.Handle(SetPath, p.set); err != nil {
			return nil, err
		}
	}

	if p.modes == nil || p.out == nil {
		return func() error {
			p.routes.Remove(ListPath)
			p.routes.Remove(SetPath)
			return nil
		}, nil
	}

	p.out.Add(plugin.Contribution{
		Place: PlaceAboveComposer,
		Kind:  plugin.KindSelect,
		Name:  "mode",
		Label: "Mode",

		// Asked for on every read, so the selector shows the mode in force now,
		// including in the moment after someone has changed it.
		Current: func() (string, []string) {
			all := p.modes.All()
			names := make([]string, 0, len(all))
			for _, m := range all {
				names = append(names, m.Name)
			}
			return p.modes.Current().Name, names
		},

		// The change comes through the seam rather than through an address, so
		// this plugin knows nothing about the interface drawing the selector —
		// and it says what happened out loud, because a setting that changes
		// silently is a setting people press twice to be sure.
		Set: func(value string) error {
			if err := p.modes.Set(value); err != nil {
				return err
			}
			p.say("mode: " + value)
			return nil
		},
	})

	return func() error { return nil }, nil
}

// say tells whichever interface is running what this plugin just did. Asking
// the tree for the capability rather than for a plugin means this works with
// any frontend that listens, and does nothing when none does.
func (p *Plugin) say(text string) {
	if notifier, ok := plugin.Find[plugin.Notify](p.tree); ok {
		notifier.Say(text)
	}
}

// modeRow is one mode as the control shows it.
type modeRow struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Current bool   `json:"current"`
}

func (p *Plugin) list(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	current := p.modes.Current().Name
	all := p.modes.All()

	rows := make([]modeRow, 0, len(all))
	for _, m := range all {
		rows = append(rows, modeRow{Name: m.Name, Summary: m.Summary, Current: m.Name == current})
	}
	_ = json.NewEncoder(w).Encode(rows)
}

func (p *Plugin) set(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "modes: POST only", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "modes: bad request", http.StatusBadRequest)
		return
	}

	known := false
	for _, m := range p.modes.All() {
		if m.Name == req.Name {
			known = true
			break
		}
	}
	if !known {
		http.Error(w, "modes: no such mode: "+req.Name, http.StatusBadRequest)
		return
	}

	if err := p.modes.Set(req.Name); err != nil {
		http.Error(w, "modes: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
