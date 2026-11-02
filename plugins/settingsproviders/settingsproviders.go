// Package settingsproviders is the Providers section of the settings panel.
//
// Lifted out of the page the same way as the others: the panel's own markup, its
// ids intact, so the row drawing, the add form and the panel's save carry on
// working without being rebuilt.
package settingsproviders

import (
	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
)

func init() {
	plugin.Register("settingsproviders", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{panes: d.Panes}, nil
	})
}

const (
	// Slot is where this section is drawn.
	Slot = "settings"

	// Title is what the panel labels the tab.
	Title = "Providers"
)

// body is the panel's own Providers section.
const body = `<h2>Providers</h2>
<p class="intro">Where model calls go. Switching takes effect on the next call, not the next restart. Anything added here is saved to a file in the harness home rather than to the config in the repo, so nothing added by a button can ever end up in a commit.</p>
<div id="providerRows"></div>
<div class="row">
  <input id="provAddName" type="text" placeholder="name" spellcheck="false">
  <input id="provAddUrl" type="text" placeholder="https://openrouter.ai/api/v1" spellcheck="false">
  <input id="provAddKey" type="text" placeholder="key: VARIABLE_NAME (optional)" spellcheck="false">
  <button id="provAddBtn" type="button">Add</button>
</div>
<div class="note-line" id="provNote"></div>`

// Plugin contributes the section.
type Plugin struct {
	panes plugin.Panes
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "settingsproviders" }

// About says what this is, so the plugins screen can group it and say what it
// is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindUI,
		What: "The Providers section of the settings panel: where model calls go, and the endpoints you have set up.",
	}
}

// Setup offers the section, and takes it back when unmounted.
func (p *Plugin) Setup() (func() error, error) {
	if p.panes != nil {
		p.panes.Add(plugin.Pane{Slot: Slot, Title: Title, Order: 40, Body: body})
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
