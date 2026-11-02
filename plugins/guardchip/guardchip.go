// Package guardchip offers the guard's mode to whatever interface is running.
//
// It is the second thing to move out of the interface and into a plugin, and
// the shape is the same as the mode selector's: read the choice off the seam
// that owns it, offer it, and apply a change through that same seam. What it
// offers is how much the guard asks before it acts — writes inside the
// workspace without being asked, every write and command approved first, or
// nothing asked at all.
package guardchip

import (
	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
)

// PlaceComposerTools is the row of controls inside the message box.
const PlaceComposerTools = "composer-tools"

func init() {
	plugin.Register("guardchip", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{consent: d.Consent, out: d.Contributions}, nil
	})
}

// Plugin contributes the consent selector.
type Plugin struct {
	consent plugin.Consent
	out     plugin.Contributions
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "guardchip" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindChip,
		What: "The control showing what the guard watches, and what it stopped.",
	}
}

// Setup offers the selector. With nothing offering consent, or nothing
// collecting contributions, it does nothing at all rather than putting a
// control on screen that could not work.
func (p *Plugin) Setup() (func() error, error) {
	if p.consent == nil || p.out == nil {
		return func() error { return nil }, nil
	}

	p.out.Add(plugin.Contribution{
		Place: PlaceComposerTools,
		Kind:  plugin.KindSelect,
		Name:  "consent",
		Label: "Approval",

		// What this setting covers, said plainly, because the label cannot say
		// it and the wrong reading is the natural one: ask puts everything to
		// you, workspace asks about the things that change something, and a
		// protected path is refused in every mode.
		Note: "Ask puts every tool call to you, including tools from plugins. Workspace asks only for writes and commands. A protected path is refused in every mode.",

		// Asked for on every read, so the selector shows what the guard is
		// doing now rather than what it was doing when this mounted.
		Current: func() (string, []string) {
			return p.consent.Mode(), p.consent.Modes()
		},

		// A change goes through the seam, and a mode the guard does not know is
		// refused by the guard rather than quietly stored.
		Set: func(value string) error { return p.consent.SetMode(value) },
	})

	return func() error { return nil }, nil
}
