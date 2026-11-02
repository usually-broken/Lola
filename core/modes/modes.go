// Package modes is the mode seam: which bundle of capability the harness is
// running with, and how to switch to another.
//
// A mode is a plugin. Its definition is registered by that plugin's package at
// init, the same way plugin factories are, so the harness knows what modes exist
// before any of them is mounted. What the mode actually does is contributed when
// its plugin is mounted — which makes switching modes a mount and an unmount
// rather than a branch in the core.
//
// The registry reads the mode in force off the tree instead of remembering it.
// What is mounted is the truth, so the config file decides where the harness
// starts, and a plugin toggled from anywhere else still reads correctly here.
package modes

import (
	"fmt"
	"sort"
	"sync"

	"github.com/usually-broken/lola/core/plugin"
)

var (
	mu      sync.RWMutex
	defined = map[string]plugin.Mode{}
)

// Define registers a mode. Called from a mode package's init().
//
// It panics on a duplicate or an incomplete definition, because two packages
// claiming one mode is a programming error rather than a runtime condition —
// the same reason plugin.Register panics.
func Define(m plugin.Mode) {
	mu.Lock()
	defer mu.Unlock()

	if m.Name == "" {
		panic("modes: Define called with an empty name")
	}
	if m.Node == "" {
		panic("modes: Define called without a node for " + m.Name)
	}
	if _, exists := defined[m.Name]; exists {
		panic("modes: duplicate definition for " + m.Name)
	}
	defined[m.Name] = m
}

// Defined lists every registered mode, in name order.
func Defined() []plugin.Mode {
	mu.RLock()
	defer mu.RUnlock()

	out := make([]plugin.Mode, 0, len(defined))
	for _, m := range defined {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// lookup finds a mode by name.
func lookup(name string) (plugin.Mode, bool) {
	mu.RLock()
	defer mu.RUnlock()
	m, ok := defined[name]
	return m, ok
}

// names lists the known mode names, for an error message.
func names() []string {
	out := make([]string, 0, len(defined))
	for name := range defined {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Mounter is the part of the plugin tree a switch needs. Kept to the calls the
// switch actually makes, so this package depends on the shape of the tree
// rather than on the tree.
type Mounter interface {
	Mounted() []string
	Mount(name string) error
	Unmount(name string) error
}

// Registry is the live mode: which one is in force, and the switch.
type Registry struct {
	mu   sync.RWMutex
	tree Mounter
}

// New builds the mode registry over the plugin tree.
func New(tree Mounter) *Registry {
	return &Registry{tree: tree}
}

// All lists every mode the harness knows about.
func (r *Registry) All() []plugin.Mode { return Defined() }

// Current is the mode in force: the one whose plugin is mounted.
func (r *Registry) Current() plugin.Mode {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.tree == nil {
		return plugin.Mode{}
	}

	mounted := r.tree.Mounted()
	for _, m := range Defined() {
		for _, name := range mounted {
			if name == m.Node {
				return m
			}
		}
	}
	return plugin.Mode{}
}

// Set switches mode: it mounts the plugin for the mode asked for, and then
// takes the other mode plugins out.
//
// The mount happens first on purpose. If it fails, nothing has changed, so a
// failed switch cannot leave the harness with no mode at all. An unknown mode
// is refused rather than guessed at.
func (r *Registry) Set(name string) error {
	m, ok := lookup(name)
	if !ok {
		return fmt.Errorf("modes: unknown mode %q (known: %v)", name, names())
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.tree == nil {
		return fmt.Errorf("modes: no plugin tree, so %q cannot be switched to", name)
	}

	if !mounted(r.tree, m.Node) {
		if err := r.tree.Mount(m.Node); err != nil {
			return fmt.Errorf("modes: mount %s: %w", m.Node, err)
		}
	}

	for _, other := range Defined() {
		if other.Node == m.Node || !mounted(r.tree, other.Node) {
			continue
		}
		if err := r.tree.Unmount(other.Node); err != nil {
			return fmt.Errorf("modes: unmount %s: %w", other.Node, err)
		}
	}

	return nil
}

// mounted reports whether a node is in the tree.
func mounted(tree Mounter, node string) bool {
	for _, name := range tree.Mounted() {
		if name == node {
			return true
		}
	}
	return false
}

// compile-time: Registry is the seam the core declares.
var _ plugin.Modes = (*Registry)(nil)
