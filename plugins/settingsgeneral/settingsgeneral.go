// Package settingsgeneral is the General section of the settings panel.
//
// This is the panel's own General, lifted out of the page: the same markup, the
// same classes, the same ids the page's save button already looks for. The page
// no longer holds a copy, so there is only ever one — which is the whole point.
package settingsgeneral

import (
	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
)

func init() {
	plugin.Register("settingsgeneral", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{panes: d.Panes}, nil
	})
}

const (
	// Slot is where this section is drawn.
	Slot = "settings"

	// Title is what the panel labels the tab.
	Title = "General"
)

// body is the panel's own General section. The ids matter: the page's save
// button and its workspace rows work on them, so the section keeps them and the
// behaviour comes along with the markup.
const body = `  <h2>General</h2>
  <p class="intro">What the harness runs against, where it may work, and where your code lives.</p>

  <div class="field">
    <div class="label">Model</div>
    <div class="why">Which model tasks run against.</div>
    <select id="setModel"></select>
  </div>

  <div class="field">
    <div class="label">Workspace</div>
    <div class="why">Directories the file tools may touch. Anything outside them is refused.</div>
    <div class="rows" id="wsRows"></div>
    <div class="addrow">
      <input id="wsAdd" type="text" placeholder="~/somewhere" spellcheck="false">
      <button id="wsAddBtn" type="button">Add</button>
    </div>
  </div>

  <div class="field">
    <div class="label">Source</div>
    <div class="why">Where your own code lives, so editing it needs approval.</div>
    <input id="setSource" type="text" spellcheck="false" placeholder="not set">
  </div>`

// Plugin contributes the section.
type Plugin struct {
	panes plugin.Panes
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "settingsgeneral" }

// About says what this is, so the plugins screen can group it and say what it
// is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindUI,
		What: "The General section of the settings panel: the model, the folders it may touch, and where code lives.",
	}
}

// Setup offers the section, and takes it back when unmounted.
func (p *Plugin) Setup() (func() error, error) {
	if p.panes != nil {
		p.panes.Add(plugin.Pane{Slot: Slot, Title: Title, Order: 10, Body: body})
	}
	return func() error {
		if p.panes != nil {
			p.panes.Remove(Title)
		}
		return nil
	}, nil
}

// Assert at compile time that this plugin satisfies the seam every plugin does.
var _ core.Plugin = (*Plugin)(nil)
