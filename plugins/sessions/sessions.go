// Package sessions keeps the harness's sessions in one place.
//
// It is the only plugin that builds a session store. The agent loop, both
// frontends and the command line ask the tree for a keeper instead, so they read
// and write the same files through the same object rather than each constructing
// their own over the same folder.
//
// Unmount it and nothing is saved, which is the honest reading of a config that
// does not have it.
package sessions

import (
	"path/filepath"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/home"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/session"
)

func init() {
	plugin.Register("sessions", func(plugin.Deps) (core.Plugin, error) {
		return &Plugin{store: session.NewStore(filepath.Join(home.SessionsDir(), session.DefaultFile))}, nil
	})
}

// Plugin offers the session store to everything that needs one.
type Plugin struct {
	store *session.Store
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "sessions" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind:  plugin.KindKeeper,
		What:  "Owns the folder conversations are saved in, and hands out the store.",
		Vital: true,
	}
}

// Setup mounts the plugin. The store needs nothing set up.
func (p *Plugin) Setup() (func() error, error) { return func() error { return nil }, nil }

// Sessions returns the store. This is what makes the plugin a session.Keeper,
// and it is the whole of what it contributes.
func (p *Plugin) Sessions() *session.Store { return p.store }

// Assert at compile time that this plugin satisfies the seam.
var _ session.Keeper = (*Plugin)(nil)
