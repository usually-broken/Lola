package main

import (
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/settings"
)

// keepSettings hands the settings view somewhere to keep its choices, and lets
// it apply the ones already made.
//
// It is here rather than inside the settings package because finding the keeper
// means asking the tree, and the tree is the harness's business. Asking for the
// capability rather than for a plugin by name is what keeps this honest: mount
// nothing that keeps, and the harness forgets between runs instead of failing to
// start.
func keepSettings(s *settings.Settings, tree plugin.Plugins) *settings.Settings {
	if keeper, ok := plugin.Find[plugin.Keeper](tree); ok {
		s.Use(keeper)
	}
	return s
}
