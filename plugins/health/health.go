// Package health serves a path of its own.
//
// It exists for two reasons. The first is that something outside the harness —
// a script on another machine, a check over Tailscale, a person with curl —
// should be able to ask what is running without loading a page. The second is
// that it is the proof a plugin can own a path: the frontend answers HTTP, but
// this path belongs to this plugin, and the frontend hands the request over
// without knowing what the answer means.
package health

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/routes"
	"github.com/usually-broken/lola/core/workspace"
)

func init() {
	plugin.Register("health", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{routes: d.Routes, tree: d.Plugins, workspace: d.Workspace, contributions: d.Contributions}, nil
	})
}

// Path is where this plugin answers.
const Path = "/api/health"

// PlaceInPage is where this plugin's line is drawn: the row the interface keeps
// for what plugins have to say.
const PlaceInPage = "above-composer"

// Plugin answers at Path, and draws a line for the interface. It holds the
// seams it needs and nothing else: no reference to a frontend, because the
// frontend is the thing that does not need to know this plugin exists.
type Plugin struct {
	routes        *routes.Registry
	tree          plugin.Plugins
	workspace     *workspace.Roots
	contributions plugin.Contributions
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "health" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindChip,
		What: "A chip saying the harness is up, and the switch used to test the tree.",
	}
}

// Setup offers this plugin's path.
//
// Nothing able to answer HTTP means there is no path to offer, which is not an
// error: this plugin simply has nothing to do in this tree. A path that could
// never work is a different matter and is reported, because a missing path is
// the sort of thing nobody notices until it matters.
func (p *Plugin) Setup() (func() error, error) {
	// Drawn where the interface keeps what plugins have to say, and built when
	// it is asked for rather than when it is offered, so the count it shows is
	// the count now.
	if p.contributions != nil {
		p.contributions.Add(plugin.Contribution{
			Place: PlaceInPage,
			Kind:  plugin.KindFragment,
			Name:  "health",
			Label: "Health",
			Draw:  p.line,
		})
	}

	if p.routes == nil {
		return func() error { return nil }, nil
	}
	if err := p.routes.Handle(Path, p.serve); err != nil {
		return nil, err
	}
	return func() error { return nil }, nil
}

// line is what the interface shows: the same answer the path gives, written
// small enough to sit among chips.
func (p *Plugin) line() string {
	mounted := 0
	if p.tree != nil {
		mounted = len(p.tree.Mounted())
	}
	return `<span class="chip" title="GET ` + Path + `">ok · ` + strconv.Itoa(mounted) + ` mounted</span>`
}

// serve says what is running: the tree that is mounted, and the roots a file
// tool may touch.
//
// Deliberately no more than that. This is a health check, not a description of
// the machine, and it answers the same for anyone who can reach the port —
// which is why it names no files, no sessions and no configuration.
func (p *Plugin) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "health: GET only", http.StatusMethodNotAllowed)
		return
	}

	body := struct {
		OK        bool     `json:"ok"`
		Mounted   []string `json:"mounted,omitempty"`
		Workspace []string `json:"workspace,omitempty"`
	}{OK: true}
	if p.tree != nil {
		body.Mounted = p.tree.Mounted()
	}
	if p.workspace != nil {
		body.Workspace = p.workspace.Dirs()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// Assert at compile time that this plugin satisfies the seam every plugin does.
var _ core.Plugin = (*Plugin)(nil)
