// Package standard is the default mode: the harness as it comes, with whatever
// the config mounted and nothing extra.
//
// It contributes no tools, and that is the point of it. A mode is a plugin that
// adds capability, so the mode that adds none is a plugin with an empty setup —
// which means "standard mode" is not a special case anywhere in the code.
package standard

import (
	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/modes"
	"github.com/usually-broken/lola/core/plugin"
)

func init() {
	plugin.Register("standard", func(plugin.Deps) (core.Plugin, error) {
		return &Plugin{}, nil
	})

	modes.Define(plugin.Mode{
		Name:    "standard",
		Summary: "The harness as it comes. Nothing beyond the tools the config mounted.",
		Node:    "standard",
	})
}

// Plugin is the mode itself: nothing to set up and nothing to take down, which
// is what makes it the one the harness starts in.
type Plugin struct{}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "standard" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind:  plugin.KindCore,
		What:  "The everyday pieces a run needs: who you are, the time, the workspace.",
		Vital: true,
	}
}

// Setup does nothing, successfully.
func (p *Plugin) Setup() (func() error, error) { return func() error { return nil }, nil }
