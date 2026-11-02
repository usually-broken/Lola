// Package composer is the act of sending, owned by a plugin rather than by the
// page.
//
// The page keeps the socket and the form. What moved here is the meaning: a
// message handed over at /api/composer/send goes through the run seam, so the
// page no longer knows what sending does, only where to post it.
//
// The box itself still lives in the page's markup, because drawing plugin
// markup that contains further plugin places — the model picker, the approval
// chip — needs a decision the page cannot make yet. Moving the box is the next
// step, not this one.
package composer

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/routes"
)

func init() {
	plugin.Register("composer", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{routes: d.Routes, tree: d.Plugins, contributions: d.Contributions}, nil
	})
}

// Path is where a message is handed over.
const Path = "/api/composer/send"

// PlaceInPage is where the box is drawn.
const PlaceInPage = "composer"

// Plugin answers at Path with somewhere to run a task, and draws the box that
// posts to it.
type Plugin struct {
	routes        *routes.Registry
	tree          plugin.Plugins
	contributions plugin.Contributions
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "composer" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindUI,
		What: "The box you type into, and the one path a message takes out.",
	}
}

// Setup offers the path, and says so plainly when there is nowhere to run a
// task from — a page that posts into nothing is worse than a page that cannot.
func (p *Plugin) Setup() (func() error, error) {
	// The box is drawn into the place the page keeps for it, and built when it
	// is asked for so the model it shows is the model now.
	if p.contributions != nil {
		p.contributions.Add(plugin.Contribution{
			Place: PlaceInPage,
			Kind:  plugin.KindFragment,
			Name:  "composer",
			Label: "Composer",
			Draw:  p.draw,
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

// draw is the box itself: a field, a place for other plugins' controls, and the
// model picker and send arrow where they have always been.
//
// It borrows the page's id because that is the hook its stylesheet already uses.
// That is the one place this plugin knows a frontend's business, and it is
// cheaper than teaching both a new name today.
func (p *Plugin) draw() string {
	return `<form id="composer" data-composer action="` + Path + `">
  <textarea name="text" rows="1" placeholder="Fire away…" autofocus></textarea>
  <div class="tools">
    <span data-place="composer-tools"></span>
    <span class="grow"></span>
    <select id="modelPick" title="Model"></select>
    <button id="send" type="submit" title="Send">&#8593;</button>
  </div>
</form>`
}

// serve takes a message and starts a task with it.
//
// It answers straight away rather than waiting: a task is watched, not
// awaited, and holding the request open would tie the page's form to the
// length of an answer.
func (p *Plugin) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "composer: POST only", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "composer: bad request", http.StatusBadRequest)
		return
	}

	text := strings.TrimSpace(req.Text)
	if text == "" {
		http.Error(w, "composer: empty message", http.StatusBadRequest)
		return
	}

	runner, ok := plugin.Find[plugin.Run](p.tree)
	if !ok {
		http.Error(w, "composer: nothing here can run a task", http.StatusServiceUnavailable)
		return
	}

	go func() { _, _ = runner.Do(context.Background(), text, nil) }()

	w.WriteHeader(http.StatusAccepted)
}

// Assert at compile time that this plugin satisfies the seam every plugin does.
var _ core.Plugin = (*Plugin)(nil)
