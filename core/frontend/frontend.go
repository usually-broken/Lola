// Package frontend is the user-interface seam. A frontend drives a session:
// it takes a task from a person, runs the agent, and shows what happened.
//
// The command-line flags are one frontend; the TUI is another, mounted as a
// plugin. A frontend is handed a Runtime — the seams it needs, plus the tree's
// own description of itself to display.
package frontend

import (
	"context"

	"github.com/usually-broken/lola/core/agent"
	"github.com/usually-broken/lola/core/guard"
	"github.com/usually-broken/lola/core/llm"
	"github.com/usually-broken/lola/core/session"
	"github.com/usually-broken/lola/core/settings"
	"github.com/usually-broken/lola/core/tools"
	"github.com/usually-broken/lola/core/workspace"
)

// Runtime is what a frontend is handed.
type Runtime struct {
	// Agent drives tasks to completion.
	Agent agent.Agent

	// Provider is the model adapter in use.
	Provider llm.Provider

	// Tools is the mounted tool registry.
	Tools *tools.Registry

	// Plugins is the mounted plugin tree, in mount order, for display.
	Plugins []string

	// Model is the model to run tasks against.
	Model string

	// ConfigPath is the config file the tree was built from, for display.
	ConfigPath string

	// Sessions is the store sessions are kept in. It comes from the tree, so a
	// frontend reads and writes the same ones the loop does rather than
	// building its own store over the same folder.
	Sessions *session.Store

	// SessionsDir is the folder saved sessions live in, for display.
	SessionsDir string

	// Workspace is the live set of roots the file tools may touch, so a
	// settings command can change it.
	Workspace *workspace.Roots

	// Guard is the live policy, so a settings command can change its source.
	Guard *guard.Rules

	// Settings is the settings seam: what this frontend can read and change
	// while the harness runs.
	Settings *settings.Settings
}

// Frontend is a user interface for the harness. Run blocks until the user is
// done, and may return an error if the interface itself fails.
type Frontend interface {
	Run(ctx context.Context, rt Runtime) error
}
