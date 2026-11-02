// Package tools is the tool registry seam. A tool declares itself with a name,
// a description, and a JSON-schema for its arguments; the registry holds the
// mounted set and runs a tool by name. Consumers — the agent loop, the model's
// tool-call handler — talk to the registry, never to a concrete tool.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ErrUnknownTool is returned when a call names a tool that is not registered.
var ErrUnknownTool = errors.New("unknown tool")

// ErrDuplicateTool is returned when two tools share a name.
var ErrDuplicateTool = errors.New("duplicate tool name")

// Tool is the contract every tool implements. Description and Schema are what
// the model is shown so it knows the tool exists and how to call it.
type Tool interface {
	// Name is the identifier the model calls. It must be unique in a registry.
	Name() string

	// Description tells the model what the tool does, in one or two sentences.
	Description() string

	// Schema is the JSON Schema of the tool's arguments, as raw JSON.
	Schema() json.RawMessage

	// Run executes the tool with the model-supplied arguments, as raw JSON,
	// and returns the result the model will see.
	Run(ctx context.Context, args json.RawMessage) (string, error)
}

// Gate decides whether a tool call may run. It is supplied by the harness and
// consulted before every Run, so the decision is made in code rather than asked
// of the model.
type Gate interface {
	// Check returns true if the call may proceed. When false, reason explains
	// why, and that reason goes back to the model in place of the result.
	Check(ctx context.Context, toolName string, args json.RawMessage) (bool, string)
}

// Registry owns the mounted tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
	gate  Gate
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// SetGate installs the gate consulted before every tool call. A nil gate means
// nothing is checked.
func (r *Registry) SetGate(g Gate) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gate = g
}

// Register adds a tool. The returned teardown removes it again, which is what
// makes a tool's registration reversible when its plugin unloads.
func (r *Registry) Register(t Tool) (func() error, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := t.Name()
	if _, exists := r.tools[name]; exists {
		return nil, fmt.Errorf("%w: %s", ErrDuplicateTool, name)
	}
	r.tools[name] = t

	return func() error {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.tools, name)
		return nil
	}, nil
}

// Get returns a registered tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Run executes the named tool with the given arguments.
//
// Arguments are normalised first: some providers return them as a JSON string
// containing JSON rather than as a JSON object, and every tool should see the
// object either way.
//
// Before the tool runs, the gate is consulted. A refusal never reaches the
// tool; the reason is returned as the result so the model can see why.
func (r *Registry) Run(ctx context.Context, name string, args json.RawMessage) (string, error) {
	t, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnknownTool, name)
	}

	args = unwrapArgs(args)

	// No reason, no run. An approval shown without one is a judgement on a
	// command rather than on an intention, and the model is told so it can say
	// why and try again.
	if _, err := Reason(args); err != nil {
		return "", err
	}

	r.mu.RLock()
	gate := r.gate
	r.mu.RUnlock()

	if gate != nil {
		ok, reason := gate.Check(ctx, name, args)
		if !ok {
			return "refused: " + reason, nil
		}
	}

	return t.Run(ctx, args)
}

// unwrapArgs turns a JSON string that contains JSON into that JSON. A model
// given a tool schema will usually send an object, but some providers pass the
// arguments through as a quoted string; both should reach the tool as the
// object the tool declared.
func unwrapArgs(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}

	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return raw // already an object, or not a string
	}
	if s == "" {
		return json.RawMessage("{}")
	}
	return json.RawMessage(s)
}

// Names returns the registered tool names, sorted.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Len returns the number of registered tools.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}
