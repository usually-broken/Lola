// Package creator is the mode where the harness can grow: the same room, plus
// the tools for adding to it.
//
// The capability is not in the mode's name, it is in what this plugin puts in
// the tool registry while it is mounted. Unmount it and the tools go with it,
// which is why switching modes is a mount and an unmount rather than a flag.
package creator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/modes"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/tools"
)

func init() {
	plugin.Register("creator", func(d plugin.Deps) (core.Plugin, error) {
		return New(d.Tools, d.Plugins), nil
	})

	modes.Define(plugin.Mode{
		Name:    "creator",
		Summary: "The harness with the tools to add to itself: see what could be mounted, and mount it.",
		Node:    "creator",
	})
}

// Plugin is the mode, and the tools it contributes.
type Plugin struct {
	registry *tools.Registry
	tree     plugin.Plugins
}

// New returns the plugin wired to a tool registry and to the plugin tree.
func New(reg *tools.Registry, tree plugin.Plugins) *Plugin {
	return &Plugin{registry: reg, tree: tree}
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "creator" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindTool,
		What: "Writes a new tool: a plugin that adds something the model can call.",
	}
}

// Setup registers the mode's tools and returns a teardown that takes all of
// them away, so unmounting the mode really does remove what it added.
func (p *Plugin) Setup() (func() error, error) {
	if p.registry == nil {
		return nil, fmt.Errorf("creator: no tool registry")
	}

	var teardowns []func() error
	toolset := []tools.Tool{&listPlugins{tree: p.tree}, &mountPlugin{tree: p.tree}}

	for _, tool := range toolset {
		teardown, err := p.registry.Register(tool)
		if err != nil {
			for i := len(teardowns) - 1; i >= 0; i-- {
				_ = teardowns[i]()
			}
			return nil, err
		}
		teardowns = append(teardowns, teardown)
	}

	return func() error {
		var firstErr error
		for i := len(teardowns) - 1; i >= 0; i-- {
			if err := teardowns[i](); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}, nil
}

// listPlugins reports what is in the tree, and what else could be.
//
// It is what makes mounting useful rather than a guess: a model cannot mount a
// plugin it has no way of knowing exists.
type listPlugins struct{ tree plugin.Plugins }

func (t *listPlugins) Name() string { return "list_plugins" }

func (t *listPlugins) Description() string {
	return "List the plugins mounted in the harness and the ones that could be mounted."
}

func (t *listPlugins) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (t *listPlugins) Run(context.Context, json.RawMessage) (string, error) {
	if t.tree == nil {
		return "", fmt.Errorf("list_plugins: the plugin tree is not available")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "mounted: %s", strings.Join(t.tree.Mounted(), ", "))

	if available := t.tree.Available(); len(available) > 0 {
		fmt.Fprintf(&b, "\ncould be mounted: %s", strings.Join(available, ", "))
	}
	return b.String(), nil
}

// mountPlugin adds one plugin to the running tree.
//
// It deliberately cannot take one out. Adding is reversible from the interface,
// and an agent should not be able to tear down the harness it is running inside.
type mountPlugin struct{ tree plugin.Plugins }

func (t *mountPlugin) Name() string { return "mount_plugin" }

func (t *mountPlugin) Description() string {
	return "Add a plugin to the running harness, by name. Reversible: it can be taken out again from the interface."
}

func (t *mountPlugin) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "The name of the plugin to mount."}
		},
		"required": ["name"]
	}`)
}

type mountArgs struct {
	Name string `json:"name"`
}

func (t *mountPlugin) Run(_ context.Context, raw json.RawMessage) (string, error) {
	var a mountArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("mount_plugin: bad arguments: %w", err)
	}
	if a.Name == "" {
		return "", fmt.Errorf("mount_plugin: name is required")
	}
	if t.tree == nil {
		return "", fmt.Errorf("mount_plugin: the plugin tree is not available")
	}

	if err := t.tree.Mount(a.Name); err != nil {
		return "", fmt.Errorf("mount_plugin: %w", err)
	}
	return "mounted " + a.Name, nil
}

var (
	_ tools.Tool = (*listPlugins)(nil)
	_ tools.Tool = (*mountPlugin)(nil)
)
