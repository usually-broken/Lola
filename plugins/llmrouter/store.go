package llmrouter

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// Store keeps the providers added from an interface, so a change made by
// clicking something survives a restart.
//
// It writes to the harness home rather than to the repo's config, which is the
// point of it: nothing a button can do writes anywhere near a file that might be
// committed. The format is the same one the config uses, so there is one thing
// to learn and the same parser reads it back.
type Store struct {
	mu    sync.Mutex
	path  string
	specs []Spec
}

// NewStore returns a store that writes to path, holding whatever was already
// read from it.
func NewStore(path string, already []Spec) *Store {
	return &Store{path: path, specs: append([]Spec(nil), already...)}
}

// Path is where this store writes, so an interface can say it out loud.
func (s *Store) Path() string { return s.path }

// All lists what has been added, in the order it was added.
func (s *Store) All() []Spec {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Spec(nil), s.specs...)
}

// Add saves a provider, replacing any entry of the same name. Adding one you
// already have is a correction rather than a mistake, so it replaces instead of
// failing.
func (s *Store) Add(sp Spec) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	replaced := false
	for i := range s.specs {
		if s.specs[i].Name == sp.Name {
			s.specs[i] = sp
			replaced = true
			break
		}
	}
	if !replaced {
		s.specs = append(s.specs, sp)
	}
	return s.save()
}

// Remove takes one out and reports whether it was there.
func (s *Store) Remove(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	kept := s.specs[:0]
	for _, sp := range s.specs {
		if sp.Name == name {
			found = true
			continue
		}
		kept = append(kept, sp)
	}
	s.specs = kept

	if !found {
		return false, nil
	}
	return true, s.save()
}

// save rewrites the file whole, which is honest for a file this small: there is
// nothing in it but the providers the interface added.
//
// It is written 0600, because a key can be written here if someone insists on
// pasting one. The environment variable name is the better habit, and stays the
// documented one.
func (s *Store) save() error {
	var b strings.Builder

	b.WriteString("# Providers added from the harness interface.\n")
	b.WriteString("# This file lives in the harness home, not in the repo, so nothing here is ever committed.\n")
	b.WriteString("# A key written here is a key written down: prefer the name of an environment variable.\n\n")
	b.WriteString("providers:\n")

	for _, sp := range s.specs {
		fmt.Fprintf(&b, "  - name: %s\n    url: %s\n", sp.Name, sp.URL)
		if sp.Key != "" {
			fmt.Fprintf(&b, "    key: %s\n", sp.Key)
		}
	}

	if err := os.WriteFile(s.path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("providers: save %s: %w", s.path, err)
	}
	return nil
}
