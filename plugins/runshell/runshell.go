// Package runshell is the run_shell tool plugin. It runs a shell command and
// returns its combined output.
package runshell

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/tools"
)

func init() {
	plugin.Register("run_shell", func(d plugin.Deps) (core.Plugin, error) {
		return New(d.Tools), nil
	})
}

const defaultTimeout = 30 * time.Second

// Plugin is the run_shell tool, mounted into the tree like any other plugin.
type Plugin struct {
	registry *tools.Registry
}

// New returns the plugin wired to a tool registry.
func New(reg *tools.Registry) *Plugin { return &Plugin{registry: reg} }

// Name identifies this plugin.
func (p *Plugin) Name() string { return "run_shell" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindTool,
		What: "Runs a shell command in the workspace.",
	}
}

// Setup registers the tool and returns its teardown.
func (p *Plugin) Setup() (func() error, error) {
	return p.registry.Register(p)
}

// Description tells the model what this tool does.
func (p *Plugin) Description() string {
	return "Run a shell command and return its combined standard output and error."
}

// Schema is the JSON Schema of the tool's arguments.
func (p *Plugin) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "The shell command to run."}
		},
		"required": ["command"]
	}`)
}

type args struct {
	Command string `json:"command"`
}

// Run executes the command and returns its output.
func (p *Plugin) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	var a args
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("run_shell: bad arguments: %w", err)
	}
	if a.Command == "" {
		return "", fmt.Errorf("run_shell: command is required")
	}

	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "sh", "-c", a.Command).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("run_shell: %w", err)
	}
	return string(out), nil
}

var _ tools.Tool = (*Plugin)(nil)
