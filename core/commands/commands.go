// Package commands is the command registry seam: the instructions a person can
// type at a frontend.
//
// A command is a name, a description, and what it does. Anything can register
// one — the frontend itself, or a plugin — and a frontend discovers whatever is
// registered rather than hardcoding a list. That is what lets a new plugin add
// a command and have the interface know about it, with nothing edited to make
// it appear.
package commands

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Command is one instruction a person can type. The name is the word after the
// slash: "new" for /new.
type Command struct {
	// Name is the command word, without the slash.
	Name string

	// Description is a one-line summary, shown in a help listing.
	Description string

	// Usage is the argument form, e.g. "/workspace <dir>". Empty means the
	// command takes no arguments.
	Usage string

	// Run performs the command. args are the words typed after the name. The
	// returned string is shown to the person; an error is shown as a failure.
	Run func(args []string) (string, error)
}

// ErrDuplicate is returned when two commands share a name.
var ErrDuplicate = fmt.Errorf("duplicate command")

// Registry holds the commands a frontend can offer.
type Registry struct {
	mu       sync.RWMutex
	commands map[string]Command
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{commands: make(map[string]Command)}
}

// Register adds a command. The name is normalised: a leading slash is dropped,
// and the name is lower-cased, so "/New" and "new" are the same command.
func (r *Registry) Register(c Command) error {
	name := Normalise(c.Name)
	if name == "" {
		return fmt.Errorf("commands: a command needs a name")
	}
	if c.Run == nil {
		return fmt.Errorf("commands: %s has no run function", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.commands[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicate, name)
	}
	c.Name = name
	r.commands[name] = c
	return nil
}

// Get returns a command by name, with or without a leading slash.
func (r *Registry) Get(name string) (Command, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.commands[Normalise(name)]
	return c, ok
}

// All returns every command, sorted by name, for a help listing.
func (r *Registry) All() []Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Command, 0, len(r.commands))
	for _, c := range r.commands {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Len returns how many commands are registered.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.commands)
}

// Normalise strips a leading slash and lower-cases a command name.
func Normalise(name string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(name), "/"))
}

// Split separates a typed line into the command name and its arguments.
// "/workspace ~/x" becomes ("workspace", ["~/x"]).
func Split(line string) (string, []string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", nil
	}
	return Normalise(fields[0]), fields[1:]
}
