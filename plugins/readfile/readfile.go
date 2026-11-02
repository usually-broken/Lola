// Package readfile is the read_file tool plugin. It reads a text file and
// returns its contents to the model.
package readfile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/tools"
	"github.com/usually-broken/lola/core/workspace"
)

func init() {
	plugin.Register("read_file", func(d plugin.Deps) (core.Plugin, error) {
		return New(d.Tools, d.Workspace), nil
	})
}

const defaultMaxBytes = 100_000

// Plugin is the read_file tool, mounted into the tree like any other plugin.
type Plugin struct {
	registry  *tools.Registry
	workspace *workspace.Roots
}

// New returns the plugin wired to a tool registry and, optionally, a set of
// allowed roots. A nil workspace means no bounding.
func New(reg *tools.Registry, ws *workspace.Roots) *Plugin {
	return &Plugin{registry: reg, workspace: ws}
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "read_file" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindTool,
		What: "Reads a file from the workspace.",
	}
}

// Setup registers the tool and returns its teardown.
func (p *Plugin) Setup() (func() error, error) {
	return p.registry.Register(p)
}

// Description tells the model what this tool does.
func (p *Plugin) Description() string {
	return "Read a text file from disk and return its contents."
}

// Schema is the JSON Schema of the tool's arguments.
func (p *Plugin) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute or relative path to the file."}
		},
		"required": ["path"]
	}`)
}

type args struct {
	Path string `json:"path"`
}

// Run reads the file and returns its contents.
func (p *Plugin) Run(_ context.Context, raw json.RawMessage) (string, error) {
	var a args
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("read_file: bad arguments: %w", err)
	}
	if a.Path == "" {
		return "", fmt.Errorf("read_file: path is required")
	}

	// What is opened must be what was judged. A relative path is resolved
	// against the workspace here, so a call cannot be checked inside the
	// workspace and then read a file outside it.
	target := a.Path
	if p.workspace != nil {
		resolved, err := p.workspace.Resolve(a.Path)
		if err != nil {
			return "", fmt.Errorf("read_file: %w", err)
		}
		target = resolved
	}

	data, err := os.ReadFile(target)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	if len(data) > defaultMaxBytes {
		return string(data[:defaultMaxBytes]) + "\n…(truncated)", nil
	}
	return string(data), nil
}

var _ tools.Tool = (*Plugin)(nil)
