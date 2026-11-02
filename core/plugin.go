// Package core holds the seams of the harness: the interfaces that plugins
// register against and the small pieces of machinery that everything else
// is built from. It is deliberately boring — the interesting part of Lola
// is the plugins, not the core.
package core

import "errors"

// Plugin is the single contract every part of the harness implements.
//
// A plugin contributes capability to the tree. Setup runs when the plugin is
// mounted; Teardown runs when it is unmounted, and must undo everything Setup
// did. The returned teardown from Setup is what makes "swap a component"
// a real operation rather than a promise: when a plugin leaves the tree,
// nothing it registered survives it.
type Plugin interface {
	// Name identifies the plugin in logs and in the registry. It should be
	// unique within a tree.
	Name() string

	// Setup mounts the plugin. It returns the plugin's own teardown function,
	// which the registry will call when the plugin is unmounted.
	Setup() (teardown func() error, err error)
}

// ErrDuplicatePlugin is returned when two plugins in the same tree share a name.
var ErrDuplicatePlugin = errors.New("duplicate plugin name")
