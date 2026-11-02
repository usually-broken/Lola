// Package providers is the seam for where model calls go: a list of
// OpenAI-compatible endpoints, and which one is in use.
//
// The point of it is that the endpoint is not fixed at startup. The runtime
// holds a registry, and the registry looks up the endpoint in force for every
// call — so changing provider takes effect on the next call rather than the
// next restart. That is what makes it possible to work against a small local
// model for plumbing and switch to a sharp one for the job in hand.
//
// Keys never live here. An entry names an endpoint and the environment variable
// its key comes from, and the key is read at the moment a call is made. So no
// key is ever written into a config file, handed to a page, or listed anywhere.
package llmrouter

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/usually-broken/lola/core/llm"
)

// Entry is one endpoint the harness can talk to.
type Entry struct {
	// Name is what a person sees: "local", "openrouter".
	Name string `json:"name"`

	// URL is the endpoint root, without /chat/completions.
	URL string `json:"url"`

	// Kind is the wire format this endpoint speaks. Empty is "openai".
	Kind string `json:"kind,omitempty"`

	// KeyFrom says where the key comes from, for a page to show: the name of an
	// environment variable, or "config" when it was written into the file. It is
	// never the key. Empty means the endpoint needs none.
	KeyFrom string `json:"keyFrom,omitempty"`

	// Key is either the variable to read the key from or the key itself, and it
	// is never serialised: a page gets names and URLs, never secrets.
	Key string `json:"-"`
}

// Builder makes the provider for an entry, given the key read for it. It is
// supplied by whoever wires the harness up, so this package does not depend on
// any particular adapter.
type Builder func(Entry, string) llm.Provider

// Registry holds the endpoints and which one is in use. It is itself a
// provider, which is what makes a switch take effect without rewiring anything.
type Registry struct {
	mu      sync.RWMutex
	entries []Entry
	current string
	build   Builder

	// store, when set, is where a provider added at runtime is saved, so the
	// change survives a restart. Nil means additions live until the harness
	// stops, which is the honest default.
	store *Store
}

// New builds the registry. The first entry is the one in use, so the config
// decides where the harness starts rather than the core guessing.
func New(build Builder, entries ...Entry) (*Registry, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("providers: none configured")
	}

	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.Name == "" {
			return nil, fmt.Errorf("providers: an entry has no name")
		}
		if e.URL == "" {
			return nil, fmt.Errorf("providers: %s has no url", e.Name)
		}
		if seen[e.Name] {
			return nil, fmt.Errorf("providers: duplicate provider %q", e.Name)
		}
		seen[e.Name] = true
	}

	return &Registry{
		entries: append([]Entry(nil), entries...),
		current: entries[0].Name,
		build:   build,
	}, nil
}

// All lists every endpoint, in the order they were configured.
func (r *Registry) All() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Entry(nil), r.entries...)
}

// Current is the name of the endpoint in use.
func (r *Registry) Current() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current
}

// Set changes which endpoint is in use. It takes effect on the next call. An
// unknown name is refused rather than guessed at, and changes nothing.
func (r *Registry) Set(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.find(name); !ok {
		return fmt.Errorf("providers: unknown provider %q (known: %v)", name, r.names())
	}
	r.current = name
	return nil
}

// Keep attaches where providers added at runtime are saved.
func (r *Registry) Keep(s *Store) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store = s
}

// Add makes a provider available, and saves it if a store is attached so it is
// still there after a restart.
//
// Adding a name that already exists is a correction rather than a mistake, so it
// replaces the entry instead of failing.
func (r *Registry) Add(sp Spec) (Entry, error) {
	if sp.Name == "" {
		return Entry{}, fmt.Errorf("providers: a provider needs a name")
	}
	if sp.URL == "" {
		return Entry{}, fmt.Errorf("providers: %s has no url", sp.Name)
	}
	e := entryFor(sp)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.store != nil {
		if err := r.store.Add(sp); err != nil {
			return Entry{}, err
		}
	}

	replaced := false
	for i := range r.entries {
		if r.entries[i].Name == e.Name {
			r.entries[i] = e
			replaced = true
			break
		}
	}
	if !replaced {
		r.entries = append(r.entries, e)
	}

	return e, nil
}

// Remove takes a provider out, and out of the store if one is attached.
//
// If the one removed was in use, the first that remains takes over, so the
// harness is never left pointing at nothing.
func (r *Registry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	kept := r.entries[:0]
	found := false
	for _, e := range r.entries {
		if e.Name == name {
			found = true
			continue
		}
		kept = append(kept, e)
	}

	if !found {
		return fmt.Errorf("providers: unknown provider %q (known: %v)", name, r.names())
	}
	if len(kept) == 0 {
		return fmt.Errorf("providers: %s is the only provider, so it cannot be removed", name)
	}

	r.entries = kept
	if r.current == name {
		r.current = r.entries[0].Name
	}

	if r.store != nil {
		if _, err := r.store.Remove(name); err != nil {
			return err
		}
	}
	return nil
}

// find looks an entry up. The caller holds the lock.
func (r *Registry) find(name string) (Entry, bool) {
	for _, e := range r.entries {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}

// names lists the known provider names, sorted. The caller holds the lock.
func (r *Registry) names() []string {
	out := make([]string, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e.Name)
	}
	sort.Strings(out)
	return out
}

// Name identifies this provider.
func (r *Registry) Name() string { return "providers" }

// Complete sends the conversation to the endpoint in use.
func (r *Registry) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	p, err := r.provider()
	if err != nil {
		return llm.Response{}, err
	}
	return p.Complete(ctx, req)
}

// Models lists what the endpoint in use offers, so a model can be chosen from a
// list. An endpoint that cannot list them returns none rather than failing.
func (r *Registry) Models(ctx context.Context) ([]string, error) {
	p, err := r.provider()
	if err != nil {
		return nil, err
	}

	lister, ok := p.(interface {
		Models(context.Context) ([]string, error)
	})
	if !ok {
		return nil, nil
	}
	return lister.Models(ctx)
}

// provider builds the provider for the endpoint in use, reading its key from
// the environment as it goes — so a key exported after startup is picked up on
// the next call rather than needing a restart.
//
// A missing key is not an error. Local endpoints need none, and inventing a
// failure for them would be worse than letting an endpoint that does need one
// say so itself.
func (r *Registry) provider() (llm.Provider, error) {
	r.mu.RLock()
	e, ok := r.find(r.current)
	build := r.build
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("providers: nothing in use")
	}
	if build == nil {
		return nil, fmt.Errorf("providers: no builder, so %q cannot be used", e.Name)
	}

	var key string
	if e.Key != "" {
		key = e.resolveKey()
	}
	return build(e, key), nil
}

// Spec is a provider as the config declares it.
type Spec struct {
	// Name is what a person sees.
	Name string

	// URL is the endpoint root, without /chat/completions.
	URL string

	// Key is either the name of an environment variable to read the key from,
	// or the key itself. Empty means the endpoint needs none.
	Key string

	// Kind is the wire format this endpoint speaks: "openai" when empty.
	Kind string
}

// Entries turns config specs into entries, working out for each one whether its
// key is a variable to read or a key to use.
func Entries(specs []Spec) ([]Entry, error) {
	out := make([]Entry, 0, len(specs))
	for _, s := range specs {
		if s.Name == "" {
			return nil, fmt.Errorf("providers: an entry has no name")
		}
		if s.URL == "" {
			return nil, fmt.Errorf("providers: %s has no url", s.Name)
		}
		out = append(out, entryFor(s))
	}
	return out, nil
}

// entryFor works out where an entry's key comes from. An environment variable
// name is letters, digits and underscores and nothing else, and a key is
// essentially never that shape — so the safe reading is also the natural one:
// `key: OPENROUTER_API_KEY` reads the environment, and `key: sk-…` is taken as
// the key itself.
func entryFor(s Spec) Entry {
	e := Entry{Name: s.Name, URL: s.URL}

	switch {
	case s.Key == "":
	case looksLikeEnvName(s.Key):
		e.Key, e.KeyFrom = s.Key, s.Key
	default:
		e.Key, e.KeyFrom = s.Key, "config"
	}

	return e
}

// resolveKey finds the key for an entry at the moment it is needed, so a
// variable exported after startup is picked up on the next call.
func (e Entry) resolveKey() string {
	switch {
	case e.Key == "":
		return ""
	case looksLikeEnvName(e.Key):
		return os.Getenv(e.Key)
	default:
		return e.Key
	}
}

// looksLikeEnvName reports whether a value is a variable's name rather than a
// key itself.
func looksLikeEnvName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		default:
			return false
		}
	}
	return true
}

// compile-time: the registry is a provider, which is the whole trick.
var _ llm.Provider = (*Registry)(nil)
