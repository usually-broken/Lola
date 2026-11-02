// Package deletefile is the delete_file tool plugin. It removes a file.
//
// It exists because without it the only way to remove a file was to reach for
// the shell, which meant the least safe route was the only one available for the
// simplest job. A delete is a write with worse consequences, so it goes through
// the same workspace check and the same consent as a write.
package deletefile

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
	plugin.Register("delete_file", func(d plugin.Deps) (core.Plugin, error) {
		return New(d.Tools, d.Workspace), nil
	})
}

// Plugin is the delete_file tool, mounted into the tree like any other plugin.
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
func (p *Plugin) Name() string { return "delete_file" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindTool,
		What: "Deletes a file from the workspace.",
	}
}

// Setup registers the tool and returns its teardown.
func (p *Plugin) Setup() (func() error, error) {
	return p.registry.Register(p)
}

// Description tells the model what this tool does.
func (p *Plugin) Description() string {
	return "Delete a file, naming it the same way read_file and write_file do. Removes files only, never folders."
}

// Schema is the JSON Schema of the tool's arguments.
func (p *Plugin) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute or relative path to the file. A relative path is inside the workspace."}
		},
		"required": ["path"]
	}`)
}

type args struct {
	Path string `json:"path"`
}

// Run removes the file and returns a short acknowledgement.
func (p *Plugin) Run(_ context.Context, raw json.RawMessage) (string, error) {
	var a args
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("delete_file: bad arguments: %w", err)
	}
	if a.Path == "" {
		return "", fmt.Errorf("delete_file: path is required")
	}

	// The path removed must be the path that was judged, or the check and the
	// delete would be talking about different files.
	target := a.Path
	if p.workspace != nil {
		resolved, err := p.workspace.Resolve(a.Path)
		if err != nil {
			return "", fmt.Errorf("delete_file: %w", err)
		}
		target = resolved
	}

	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("delete_file: %w", err)
	}
	// Folders are not this tool's business: removing one takes everything
	// inside it, and that is a different decision from deleting a file.
	if info.IsDir() {
		return "", fmt.Errorf("delete_file: %s is a folder, and this deletes files only", a.Path)
	}

	if err := os.Remove(target); err != nil {
		return "", fmt.Errorf("delete_file: %w", err)
	}

	return fmt.Sprintf("deleted %s", target), nil
}

var _ tools.Tool = (*Plugin)(nil)
