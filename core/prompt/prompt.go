// Package prompt is where the harness assembles what the model is told before
// anything else.
//
// The loop used to own a constant. That worked while there was one thing to say,
// and stopped working the moment a second thing needed to say it — voice mode
// asking for brevity, a briefing saying what is outstanding, a persona, a memory.
// So the text is assembled rather than written: each part names itself, says where
// it wants to sit, and contributes.
//
// Parts are ordered, not concatenated by whoever got there first, because a prompt
// is read top to bottom and the order is part of the meaning.
package prompt

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Part is one contribution to the prompt.
type Part struct {
	// Name identifies the contributor, and is how a part is replaced or removed.
	// Two parts with the same name are a mistake, not a merge.
	Name string `json:"name"`

	// Order places it: lower comes first. Equal orders fall back to name, so the
	// result never depends on mount order.
	Order int `json:"order"`

	// Text is what the model is told.
	Text string `json:"text"`

	// Live, when set, is asked for the text each time the prompt is assembled
	// instead of Text being used. A part that describes something which does not
	// exist yet — the mode in force, whether voice is on — has to be able to
	// answer later rather than at the moment it is added.
	Live func() string `json:"-"`

	// About is one line for a screen, so a person can see why something is in
	// their prompt rather than having to read the harness.
	About string `json:"about,omitempty"`
}

// Registry is the set of parts currently contributing.
type Registry struct {
	mu    sync.Mutex
	parts map[string]Part
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{parts: map[string]Part{}}
}

// Add puts a part in, refusing a duplicate name.
//
// Replacing silently would mean two plugins claiming the same corner of the prompt
// and one of them losing without knowing, which is far harder to debug than an
// error at mount time.
func (r *Registry) Add(part Part) error {
	name := strings.TrimSpace(part.Name)
	if name == "" {
		return fmt.Errorf("prompt: a part needs a name")
	}
	if part.Live == nil && strings.TrimSpace(part.Text) == "" {
		return fmt.Errorf("prompt: %q contributes nothing", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, taken := r.parts[name]; taken {
		return fmt.Errorf("prompt: %q already contributes", name)
	}
	part.Name = name
	r.parts[name] = part
	return nil
}

// Remove takes a part back. It reports whether anything was there.
func (r *Registry) Remove(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.parts[name]; !ok {
		return false
	}
	delete(r.parts, name)
	return true
}

// resolved is the text a part contributes right now.
func (p Part) resolved() string {
	if p.Live != nil {
		return strings.TrimSpace(p.Live())
	}
	return strings.TrimSpace(p.Text)
}

// Parts returns everything contributing, in the order it will be read, with any
// live text filled in.
func (r *Registry) Parts() []Part {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Part, 0, len(r.parts))
	for _, part := range r.parts {
		part.Text = part.resolved()
		out = append(out, part)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Text is the assembled prompt. Empty when nothing is contributing, which the
// caller should treat as "no system prompt" rather than "an empty one".
func (r *Registry) Text() string {
	parts := r.Parts()
	if len(parts) == 0 {
		return ""
	}

	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		if text := part.resolved(); text != "" {
			lines = append(lines, text)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n\n")
}

// Len is how many parts are contributing.
func (r *Registry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.parts)
}
