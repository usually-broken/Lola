// Package adapters holds the wire formats plugins have offered.
//
// It is the counterpart to the tool and command registries: a plugin that knows
// how to talk to some API offers that knowledge, and the router asks for it by
// name. Nothing here knows what any shape looks like.
package adapters

import (
	"sort"
	"sync"

	"github.com/usually-broken/lola/core/llm"
	"github.com/usually-broken/lola/core/plugin"
)

// Registry holds the kinds offered, by name.
type Registry struct {
	mu   sync.RWMutex
	kind map[string]plugin.Kind
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{kind: make(map[string]plugin.Kind)}
}

// Add offers a shape. A shape with no name is ignored: it could never be asked
// for. Offering a name twice replaces the earlier one, so the last mounted wins.
func (r *Registry) Add(k plugin.Kind) {
	if k == nil || k.Kind() == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.kind[k.Kind()] = k
}

// For answers with the shape of this name, if one is mounted.
func (r *Registry) For(kind string) (llm.Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	k, ok := r.kind[kind]
	if !ok {
		return nil, false
	}
	return k, true
}

// Kinds lists what is mounted, sorted, for a screen or an error message.
func (r *Registry) Kinds() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]string, 0, len(r.kind))
	for name := range r.kind {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// compile-time: a Registry is what Deps wants.
var _ plugin.Adapters = (*Registry)(nil)
