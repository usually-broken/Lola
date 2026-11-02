// Package settingspermissions is the Permissions section of the settings panel.
//
// Lifted out of the page the same way as General: the panel's own markup, the
// panel's own ids, so the guard lists it draws and the page's save button keep
// working without being rebuilt.
package settingspermissions

import (
	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
)

func init() {
	plugin.Register("settingspermissions", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{panes: d.Panes}, nil
	})
}

const (
	// Slot is where this section is drawn.
	Slot = "settings"

	// Title is what the panel labels the tab.
	Title = "Permissions"
)

// body is the panel's own Permissions section, ids and all.
const body = `  <h2>Permissions</h2>
  <p class="intro">What the agent may do without asking. Checked in code before any tool call runs.</p>

  <div class="field">
    <div class="label">Never touched</div>
    <div class="why">Refused outright, whatever the answer.</div>
    <div class="rows" id="protectedRows"></div>
    <div class="addrow">
      <input id="protectedAdd" type="text" placeholder="~/path" spellcheck="false">
      <button id="protectedAddBtn" type="button">Add</button>
    </div>
  </div>

  <div class="field">
    <div class="label">Asks before writing</div>
    <div class="why">A question appears and waits for a yes or no.</div>
    <div class="rows" id="gatedRows"></div>
    <div class="addrow">
      <input id="gatedAdd" type="text" placeholder="~/path" spellcheck="false">
      <button id="gatedAddBtn" type="button">Add</button>
    </div>
  </div>

  <div class="field">
    <div class="label">Asks before running</div>
    <div class="why">Shell commands that need a yes first.</div>
    <div class="rows" id="dangerousRows"></div>
    <div class="addrow">
      <input id="dangerousAdd" type="text" placeholder="rm" spellcheck="false">
      <button id="dangerousAddBtn" type="button">Add</button>
    </div>
  </div>`

// Plugin contributes the section.
type Plugin struct {
	panes plugin.Panes
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "settingspermissions" }

// About says what this is, so the plugins screen can group it and say what it
// is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindUI,
		What: "The Permissions section of the settings panel: what the agent may never touch, must ask about, and may not do at all.",
	}
}

// Setup offers the section, and takes it back when unmounted.
func (p *Plugin) Setup() (func() error, error) {
	if p.panes != nil {
		p.panes.Add(plugin.Pane{Slot: Slot, Title: Title, Order: 20, Body: body})
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
