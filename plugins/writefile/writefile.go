// Package writefile is the write_file tool plugin. It writes text to a file on
// disk.
package writefile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/tools"
	"github.com/usually-broken/lola/core/workspace"
)

func init() {
	plugin.Register("write_file", func(d plugin.Deps) (core.Plugin, error) {
		return New(d.Tools, d.Workspace), nil
	})
}

// Plugin is the write_file tool, mounted into the tree like any other plugin.
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
func (p *Plugin) Name() string { return "write_file" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindTool,
		What: "Writes a file in the workspace.",
	}
}

// Setup registers the tool and returns its teardown.
func (p *Plugin) Setup() (func() error, error) {
	return p.registry.Register(p)
}

// Description tells the model what this tool does.
func (p *Plugin) Description() string {
	return "Write text to a file on disk, creating parent directories as needed."
}

// Schema is the JSON Schema of the tool's arguments.
func (p *Plugin) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute or relative path to the file."},
			"content": {"type": "string", "description": "The text to write."}
		},
		"required": ["path", "content"]
	}`)
}

type args struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Run writes the file and returns a short acknowledgement.
func (p *Plugin) Run(_ context.Context, raw json.RawMessage) (string, error) {
	var a args
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("write_file: bad arguments: %w", err)
	}
	if a.Path == "" {
		return "", fmt.Errorf("write_file: path is required")
	}

	// What is written must be what was judged. A relative path is resolved
	// against the workspace here, so a call cannot be checked inside the
	// workspace and then written outside it.
	target := a.Path
	if p.workspace != nil {
		resolved, err := p.workspace.Resolve(a.Path)
		if err != nil {
			return "", fmt.Errorf("write_file: %w", err)
		}
		target = resolved
	}

	if dir := filepath.Dir(target); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("write_file: create parent directory: %w", err)
		}
	}
	if err := os.WriteFile(target, []byte(a.Content), 0o644); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}

	return fmt.Sprintf("wrote %d bytes to %s", len(a.Content), target), nil
}

var _ tools.Tool = (*Plugin)(nil)
