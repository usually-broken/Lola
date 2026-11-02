// Package panes holds the panels plugins offer the interface.
//
// It is the counterpart to the tool and command registries: a plugin that has
// something worth showing contributes a pane, and whatever frontend is running
// decides how to draw it. Nothing here knows what a pane looks like.
package panes

import (
	"sync"

	"github.com/usually-broken/lola/core/plugin"
)

// Registry holds the panes plugins have contributed, in the order they were
// added, so a later plugin's panel appears below an earlier one's rather than
// somewhere arbitrary.
type Registry struct {
	mu    sync.RWMutex
	panes []plugin.Pane
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{}
}

// Add contributes a pane. A pane with no slot or no title is ignored: an
// interface cannot place or label it, and a blank panel is worse than none.
func (r *Registry) Add(pane plugin.Pane) {
	if pane.Slot == "" || pane.Title == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.panes = append(r.panes, pane)
}

// All lists the contributed panes in the order they were added.
func (r *Registry) All() []plugin.Pane {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// An empty list rather than nil, so a frontend receives [] instead of
	// null and does not have to guard against it.
	if r.panes == nil {
		return []plugin.Pane{}
	}
	return append([]plugin.Pane(nil), r.panes...)
}

// Remove takes back every pane with this title, so a section switched off stops
// being drawn.
func (r *Registry) Remove(title string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	kept := r.panes[:0]
	for _, pane := range r.panes {
		if pane.Title != title {
			kept = append(kept, pane)
		}
	}
	r.panes = kept
}

// compile-time: a Registry is what Deps wants.
var _ plugin.Panes = (*Registry)(nil)
