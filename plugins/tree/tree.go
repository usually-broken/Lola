// Package tree lets the running harness be rearranged from the page.
//
// The tree exposed itself to the frontend through the web plugin, which meant
// the frontend needed code to know what a plugin is. This owns both halves
// instead: the list, and the switching. Mounting is a change to the running
// tree only — the config file is untouched, so a switch flipped here cannot
// quietly change what starts next time.
package tree

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/routes"
)

func init() {
	plugin.Register("tree", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{routes: d.Routes, tree: d.Plugins, panes: d.Panes}, nil
	})
}

const (
	// ListPath says what is mounted and what could be.
	ListPath = "/api/tree"

	// SetPath adds one, or takes one out, while the harness runs.
	SetPath = "/api/tree/set"

	// HTMLPath renders the settings section: the list, grouped, ready to draw.
	HTMLPath = "/api/tree/html"
)

// Plugin owns the two paths and holds the tree it acts on.
type Plugin struct {
	routes *routes.Registry
	tree   plugin.Plugins
	panes  plugin.Panes
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "tree" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindUI,
		What: "The list of plugins, and the switches that change the tree while it runs.",
	}
}

// Setup offers both paths. Nothing able to answer HTTP means there is nothing
// to offer; a tree that is not exposed means the paths answer honestly that
// there is no tree to show, rather than pretending to be empty.
func (p *Plugin) Setup() (func() error, error) {
	if p.panes != nil {
		p.panes.Add(plugin.Pane{Slot: "settings", Title: "Plugins", Order: 30, Source: HTMLPath})
	}

	if p.routes == nil {
		return func() error { return nil }, nil
	}
	if err := p.routes.Handle(ListPath, p.list); err != nil {
		return nil, err
	}
	if err := p.routes.Handle(SetPath, p.set); err != nil {
		return nil, err
	}
	if err := p.routes.Handle(HTMLPath, p.html); err != nil {
		return nil, err
	}

	return func() error {
		p.routes.Remove(HTMLPath)
		if p.panes != nil {
			p.panes.Remove("Plugins")
		}
		return nil
	}, nil
}

// html draws the section itself: the switches, grouped by what kind of thing
// each plugin is, with the sentence each plugin gave about itself. It is built
// here rather than in the page because only the tree knows any of it.
func (p *Plugin) html(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "tree: this section is a GET", http.StatusMethodNotAllowed)
		return
	}
	if p.tree == nil {
		http.Error(w, "tree: the plugin tree is not available", http.StatusServiceUnavailable)
		return
	}

	rows := p.described(p.tree.Mounted())
	on := make(map[string]bool, len(rows))
	for _, row := range rows {
		on[row.Name] = true
	}
	for _, name := range p.tree.Available() {
		if !on[name] {
			rows = append(rows, entry{Name: name, Kind: "other"})
		}
	}

	order := []string{"ui", "voice", "prompt", "chip", "tool", "keeper", "model", "core", "demo", "other"}
	grouped := make(map[string][]entry)
	for _, row := range rows {
		kind := row.Kind
		if kind == "" {
			kind = "other"
		}
		known := false
		for _, k := range order {
			if k == kind {
				known = true
			}
		}
		if !known {
			kind = "other"
		}
		grouped[kind] = append(grouped[kind], row)
	}

	// Group headings are words, not the names of the constants behind them.
	heading := map[string]string{
		"ui":     "Interface",
		"prompt": "Prompt",
		"voice":  "Voice",
		"chip":   "Chips",
		"tool":   "Tools",
		"keeper": "Keepers",
		"model":  "Models",
		"core":   "Core",
		"demo":   "Demo",
		"other":  "Not described",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	var b bytes.Buffer
	b.WriteString(`<h2>Plugins</h2><p class="intro">What is in the tree, and the switches that change it while the harness runs. Nothing here is written to the config: what you switch off comes back on the next start.</p>`)
	for _, kind := range order {
		items := grouped[kind]
		if len(items) == 0 {
			continue
		}

		label := heading[kind]
		if label == "" {
			label = kind
		}
		fmt.Fprintf(&b, "<div class=\"group-heading\" data-group=\"%s\">%s <span class=\"count\">(%d)</span></div>", kind, label, len(items))
		for _, item := range items {
			state := ""
			title := "not mounted — click to add it"
			onFlag := "0"
			if on[item.Name] {
				state = " on"
				onFlag = "1"
				title = "mounted — click to take it out"
			}
			if item.Vital {
				title = "this one probably should stay — click anyway to take it out"
			}

			fmt.Fprintf(&b, "<div class=\"switchrow\"><button type=\"button\" class=\"switch%s\" data-switch=\"%s\" data-on=\"%s\" title=\"%s\"></button><span class=\"switchname\">%s",
				state, item.Name, onFlag, title, item.Name)
			if item.What != "" {
				fmt.Fprintf(&b, "<span class=\"meta\">%s</span>", item.What)
			}
			b.WriteString("</span></div>")
		}
	}

	_, _ = w.Write(b.Bytes())
}

func (p *Plugin) list(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if p.tree == nil {
		_, _ = w.Write([]byte(`{"mounted":[],"available":[]}`))
		return
	}

	_ = json.NewEncoder(w).Encode(struct {
		Mounted   []entry  `json:"mounted"`
		Available []string `json:"available"`
	}{
		Mounted:   p.described(p.tree.Mounted()),
		Available: p.tree.Available(),
	})
}

// entry is one mounted plugin as the screen needs it: what it is called, what
// sort of thing it is, what it is for, and whether it may be taken out.
type entry struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	What  string `json:"what"`
	Vital bool   `json:"vital"`
}

// described asks each mounted plugin what it is. A plugin that says nothing is
// listed as an unclassified one rather than left out: the list is of what is
// mounted, not of what has been described.
func (p *Plugin) described(names []string) []entry {
	out := make([]entry, 0, len(names))
	for _, name := range names {
		e := entry{Name: name, Kind: "other"}

		if inst, ok := p.tree.Get(name); ok {
			if d, ok := inst.(plugin.Described); ok {
				about := d.About()
				if about.Kind != "" {
					e.Kind = about.Kind
				}
				e.What = about.What
				e.Vital = about.Vital
			}
		}

		out = append(out, e)
	}
	return out
}

func (p *Plugin) set(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "tree: POST only", http.StatusMethodNotAllowed)
		return
	}
	if p.tree == nil {
		http.Error(w, "tree: the plugin tree is not available", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Name string `json:"name"`
		On   bool   `json:"on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "tree: bad request", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "tree: no plugin named", http.StatusBadRequest)
		return
	}

	var err error
	if req.On {
		err = p.tree.Mount(req.Name)
	} else {
		// Vital is advice, not a lock. The plugin says it should stay, the
		// screen says so too, and the switch still works — this is your tree,
		// and a harness that refuses to be rearranged is not one the claim
		// "everything is a plugin" can be made about.
		err = p.tree.Unmount(req.Name)
	}
	if err != nil {
		http.Error(w, "tree: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// Assert at compile time that this plugin satisfies the seam every plugin does.
var _ core.Plugin = (*Plugin)(nil)
