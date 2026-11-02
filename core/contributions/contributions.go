// Package contributions holds what plugins offer an interface.
//
// It is a list and nothing else: the frontend decides which places exist and
// how each kind is drawn, so this package is the same for a browser and for a
// terminal.
package contributions

import (
	"sync"

	"github.com/usually-broken/lola/core/plugin"
)

// Registry collects contributions in the order they are offered.
type Registry struct {
	mu    sync.Mutex
	items []plugin.Contribution
}

// New returns an empty registry.
func New() *Registry { return &Registry{} }

// Add offers a contribution. A contribution naming no place, or no kind, is
// refused rather than stored: neither can be rendered, and a silent no-show in
// an interface is a miserable thing to debug.
func (r *Registry) Add(c plugin.Contribution) {
	if c.Place == "" || c.Kind == "" || c.Name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, c)
}

// All lists the contributions offered, in the order they were offered.
//
// Anything that can change is asked for its value here, so what is returned is
// true at the moment it is asked for rather than at the moment it was offered.
func (r *Registry) All() []plugin.Contribution {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]plugin.Contribution, 0, len(r.items))
	for _, c := range r.items {
		if c.Current != nil {
			value, options := c.Current()
			c.Value = value
			c.Options = options
		}
		if c.Draw != nil {
			c.Value = c.Draw()
		}
		c.Changeable = c.Set != nil
		out = append(out, c)
	}
	return out
}

// Assert at compile time that this satisfies the seam.
var _ plugin.Contributions = (*Registry)(nil)
