// Package greet is the example plugin. It does nothing useful: it exists so a
// reader can open one small, complete plugin and see the entire contract.
//
// To make your own plugin, copy this folder, rename it, and fill in the two
// methods on the Plugin type below.
package greet

import (
	"fmt"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
)

func init() {
	plugin.Register("greet", func(plugin.Deps) (core.Plugin, error) {
		return &Plugin{}, nil
	})
}

// Plugin prints a line when it joins the tree and another when it leaves.
type Plugin struct{}

// Name identifies this plugin in the registry.
func (p *Plugin) Name() string { return "greet" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: "demo",
		What: "Says hello. A demonstration that a plugin can be mounted and speak from the tree.",
	}
}

// Setup mounts the plugin and returns its teardown.
func (p *Plugin) Setup() (func() error, error) {
	fmt.Println("greet: hello from the tree")

	return func() error {
		fmt.Println("greet: goodbye from the tree")
		return nil
	}, nil
}

// Assert at compile time that Plugin satisfies the contract.
var _ core.Plugin = (*Plugin)(nil)
