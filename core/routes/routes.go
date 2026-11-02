// Package routes holds the paths plugins asked to serve.
//
// A frontend owns the socket, but a feature belongs to the plugin that offers
// it. This is the seam between the two: a plugin offers a path, whatever can
// answer HTTP mounts it, and the plugin keeps ownership of what the path
// means. Nothing in the frontend has to learn what a plugin's path is for.
//
// Matching is done here rather than by the frontend, so every frontend agrees
// about which path a request belongs to: a pattern ending in a slash covers
// everything under it, anything else is exact, and the longest pattern wins.
package routes

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// Route is one path a plugin asked to serve.
type Route struct {
	// Pattern is the path, in the shape net/http uses.
	Pattern string

	// Handler answers it.
	Handler http.HandlerFunc
}

// Registry is the set of paths plugins have offered.
type Registry struct {
	mu     sync.RWMutex
	routes []Route
}

// New returns an empty registry.
func New() *Registry { return &Registry{} }

// Handle is where a plugin offers a path.
//
// It refuses one that could never work rather than accepting it and answering
// 404 later: an empty pattern, one without a leading slash, or no handler at
// all. A refused offer is the plugin's own error to report — Setup has an error
// to return for exactly this — because a path that silently goes missing is
// the sort of thing nobody notices until it matters.
func (r *Registry) Handle(pattern string, h http.HandlerFunc) error {
	if pattern == "" {
		return fmt.Errorf("routes: a path is needed")
	}
	if !strings.HasPrefix(pattern, "/") {
		return fmt.Errorf("routes: %q does not start with a slash", pattern)
	}
	if h == nil {
		return fmt.Errorf("routes: %q has no handler", pattern)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = append(r.routes, Route{Pattern: pattern, Handler: h})
	return nil
}

// Remove takes a path back, so a plugin that unmounts stops answering on it.
//
// Without this the registry only ever grows: a plugin switched off in the
// running tree keeps serving its path, which reads as a switch that does
// nothing. Ownership has to be reversible everywhere, or "unmounted" means
// nothing more than "not listed".
func (r *Registry) Remove(pattern string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	kept := r.routes[:0]
	for _, route := range r.routes {
		if route.Pattern != pattern {
			kept = append(kept, route)
		}
	}
	r.routes = kept
}

// All is everything offered, longest pattern first, which is the order they are
// matched in.
func (r *Registry) All() []Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Route, len(r.routes))
	copy(out, r.routes)
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i].Pattern) > len(out[j].Pattern)
	})
	return out
}

// Match finds the handler for a path, if a plugin offered one.
func (r *Registry) Match(path string) (http.HandlerFunc, bool) {
	for _, route := range r.All() {
		if strings.HasSuffix(route.Pattern, "/") {
			if strings.HasPrefix(path, route.Pattern) {
				return route.Handler, true
			}
			continue
		}
		if path == route.Pattern {
			return route.Handler, true
		}
	}
	return nil, false
}

// Serve answers with a plugin's handler when one claims the path, and hands
// everything else to next.
//
// Looked up per request rather than mounted once, so a plugin may offer a path
// at any point — including after the frontend has started — and does not have
// to be built before the frontend is. A frontend with nothing offered behaves
// exactly as it did before this seam existed, which is what makes it safe to
// put in front of a working server.
func (r *Registry) Serve(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if h, ok := r.Match(req.URL.Path); ok {
			h(w, req)
			return
		}
		next.ServeHTTP(w, req)
	})
}
