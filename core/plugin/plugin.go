// Package plugin defines the runtime side of the plugin system: the factory
// registry that turns a plugin name into a mounted plugin.
//
// A plugin package registers a factory in its init():
//
//	func init() { plugin.Register("greet", func(d plugin.Deps) (core.Plugin, error) {
//		return &Plugin{}, nil
//	}) }
//
// The config loader then builds the tree by name, so no code decides what is
// mounted — the config file does.
package plugin

import (
	"fmt"
	"sort"
	"sync"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/commands"
	"github.com/usually-broken/lola/core/prompt"
	"github.com/usually-broken/lola/core/routes"
	"github.com/usually-broken/lola/core/tools"
	"github.com/usually-broken/lola/core/workspace"
)

// Deps is what a factory is handed when it builds a plugin. It carries the
// seams a plugin might need to register against. Adding a seam here is how a
// new kind of plugin gets what it needs, without changing the registry.
type Deps struct {
	// Tools is the tool registry, for plugins that contribute tools.
	Tools *tools.Registry

	// Workspace is the roots a plugin's file tools may touch. Nil means no
	// bounding is configured.
	Workspace *workspace.Roots

	// Commands is the command registry, for plugins that contribute commands
	// the interface can offer.
	Commands *commands.Registry

	// Plugins is the tree itself: what is mounted, what could be, and how to
	// change it. Nil when the tree is not exposed, which is the default.
	Plugins Plugins

	// Panes is where a plugin offers a panel to the interface. Nil means the
	// interface is not accepting them, which is the default.
	Panes Panes

	// Contributions is where a plugin offers things to the interface — a chip,
	// a toggle, a selector, a panel. Nil means nothing is collecting them.
	Contributions Contributions

	// Consent is how much the guard asks before it acts. Nil means nothing is
	// offering a choice of it.
	Consent Consent

	// Modes is which bundle of capability the harness is running with, and how
	// to switch. Nil means nothing is offering a choice, which is the default.
	Modes Modes

	// Routes is where a plugin offers a path of its own for whatever can answer
	// HTTP to serve. Nil means nothing can, in which case a plugin with a path
	// to offer simply has nothing to do — not an error, and not a silent
	// failure either, because it never thought it had one.
	//
	// This is how a feature moves out of a frontend: the frontend keeps the
	// socket, the plugin keeps the meaning.
	Routes *routes.Registry

	// Sessions is the store conversations are kept in. Nil means nothing is
	// offering conversations, which a plugin listing them should say plainly
	// rather than pretend is an empty list.
	Sessions Sessions

	// Settings is what the harness is set to, for a screen that offers to
	// change it. Nil means nothing is offering settings.
	Settings Settings

	// Adapters is the set of wire formats mounted, so a router can find the one
	// an endpoint asked for instead of assuming a shape.
	Adapters Adapters

	// Prompt is what the model is told before anything else. Nil means nothing is
	// assembling one, in which case the loop falls back to its own text rather
	// than sending an empty prompt — a harness with no prompt is worse than one
	// with a boring prompt.
	Prompt *prompt.Registry
}

// Pane is a panel a plugin contributes to the interface: where it goes, what
// it is called, and where the interface can fetch what it shows.
//
// Deliberately not a component. A frontend here is a page, a terminal, or
// something not written yet, so a pane is described rather than drawn: the
// frontend decides how to render it, and a plugin that says nothing about
// drawing still gets a panel.
type Pane struct {
	// Slot is the place it goes: "details" for now.
	Slot string `json:"slot"`

	// Title is what the interface labels it.
	Title string `json:"title"`

	// Order is where it wants to sit among panes in the same slot. Lower comes
	// first; equal keeps the order they were offered in.
	Order int `json:"order,omitempty"`

	// Rows is what it shows: label and value pairs the frontend lays out
	// however suits it.
	Rows []Row `json:"rows,omitempty"`

	// Source is a path that renders this pane's markup when it is drawn, for a
	// section whose content changes while the panel is open. Body is for a
	// section that does not change.
	Source string `json:"source,omitempty"`

	// Body is the markup for a panel, when the panel is not made of rows. It is
	// drawn inside the pane's own container, so the classes it uses are the
	// frontend's own and it looks like the place it appears in.
	Body string `json:"body,omitempty"`

	// Fields is a section's controls, described rather than drawn. A frontend
	// renders them with its own markup, so a section offered by a plugin looks
	// like the panel it appears in rather than like a form bolted onto it.
	Fields []Field `json:"fields,omitempty"`
}

// Field is one control in a section.
//
// Described, not drawn: a plugin that supplied its own HTML could not match the
// panel's styling, and a section that looks like a different program is worse
// than one that looks like nothing.
type Field struct {
	// Key is the setting it edits, and the name posted when saving.
	Key string `json:"key"`

	// Label is the line above it.
	Label string `json:"label"`

	// Why is one sentence saying what changing it does.
	Why string `json:"why,omitempty"`

	// Kind is "text", "select" or "list". Empty means "text".
	Kind string `json:"kind,omitempty"`

	// Options is for a select. Empty means the frontend fills them itself,
	// which is what a model list wants.
	Options []string `json:"options,omitempty"`

	// Placeholder is the hint shown in an empty box.
	Placeholder string `json:"placeholder,omitempty"`
}

// Row is one line in a pane.
type Row struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Panes is where plugins offer panels. An interface on Deps for the same
// reason as the rest: whatever draws the page depends on the shape, not on
// the package that holds the list.
type Panes interface {
	// Add contributes a pane. Called by a plugin during its setup.
	Add(pane Pane)

	// All lists the panes contributed, in the order they were added.
	All() []Pane

	// Remove takes back the panes a piece of interface offered, by the title it
	// gave them. A section switched off in the running tree should stop being
	// drawn, rather than linger until the next restart.
	Remove(title string)
}

// Mode is one way the harness can be configured to behave: a named bundle of
// capability, contributed by a plugin rather than decided by the core.
//
// Deliberately not a flag. What a mode *is* is the tools its plugin puts in the
// registry while it is mounted, so adding a mode is adding a plugin, and
// switching a mode is mounting one and unmounting another.
type Mode struct {
	// Name is what a person sees: "standard", "creator".
	Name string `json:"name"`

	// Summary is one line saying what the mode is for.
	Summary string `json:"summary"`

	// Node is the plugin that has to be mounted for the mode to be in force.
	// Separate from Name so a mode can be called something a person reads while
	// its plugin is called something the tree reads.
	Node string `json:"node"`
}

// Consent is how much the guard asks before it acts, as a plugin offering a
// control for it needs it: the mode in force, the modes that can be chosen, and
// how to change it.
//
// One interface on Deps, like the rest, so a plugin offering a control knows
// nothing about the guard's rules, and the guard knows nothing about what is
// drawing it.
type Consent interface {
	// Mode is the mode in force.
	Mode() string

	// Modes lists the modes that can be chosen, in the order they loosen.
	Modes() []string

	// SetMode changes it. A mode the guard does not know is refused rather than
	// stored.
	SetMode(name string) error
}

// Modes is which mode the harness is in, and how to change it.
//
// One mode is active at a time, and the seam owns the move rather than the
// caller: mounting one plugin and unmounting the other are a single step, so
// there is never a moment with two modes mounted or none.
type Modes interface {
	// All lists every mode the harness knows about.
	All() []Mode

	// Current is the mode in force.
	Current() Mode

	// Set switches to a mode. An unknown name is refused rather than guessed
	// at, and a switch that fails leaves the mode already in force untouched.
	Set(name string) error
}

// Plugins is the plugin tree as something else sees it: a frontend that wants
// to show what the harness is made of, and change it while it runs.
//
// It is an interface on Deps rather than a concrete registry so that whatever
// draws an interface depends on the shape and not on the package that owns the
// tree. It is also the only way a plugin can reach the tree, which keeps the
// rule intact: nothing gets the tree except the thing mounting plugins.
type Plugins interface {
	// Mounted lists the plugins currently in the tree, in mount order.
	Mounted() []string

	// Available lists every plugin that could be mounted, sorted.
	Available() []string

	// Get returns a mounted plugin by name, so something can ask what a plugin
	// actually is rather than only what it is called.
	Get(name string) (core.Plugin, bool)

	// Mount builds a plugin by name and adds it to the running tree.
	Mount(name string) error

	// Unmount removes a plugin from the running tree and leaves the rest.
	Unmount(name string) error
}

// Find returns the first mounted plugin that satisfies T: the capability that
// was asked for, rather than the name of whichever plugin happens to provide it.
//
// That distinction is the whole point. A harness whose binary names its plugins
// has to be edited to change them, and cannot be added to by anything running
// inside it. Asking for a capability means the parts of the tree can find each
// other, and new ones can appear without the binary knowing.
//
// Finding nothing is a miss rather than an error: the caller knows what it was
// looking for and can say so in its own words.
func Find[T any](p Plugins) (T, bool) {
	var zero T
	if p == nil {
		return zero, false
	}
	for _, name := range p.Mounted() {
		inst, ok := p.Get(name)
		if !ok {
			continue
		}
		if found, ok := inst.(T); ok {
			return found, true
		}
	}
	return zero, false
}

// Factory builds one plugin from Deps.
type Factory func(Deps) (core.Plugin, error)

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

// Register adds a factory under a name. Called from a plugin package's init().
// It panics on a duplicate name, because two packages claiming the same plugin
// name is a programming error, not a runtime condition.
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()

	if name == "" {
		panic("plugin: Register called with an empty name")
	}
	if f == nil {
		panic("plugin: Register called with a nil factory for " + name)
	}
	if _, exists := factories[name]; exists {
		panic("plugin: duplicate registration for " + name)
	}
	factories[name] = f
}

// Build looks up a factory by name and builds the plugin.
//
// The deps it hands over are not quite the ones the tree holds. Anything a
// plugin offers gets wrapped first, so that the harness knows which plugin
// offered it — which is the difference between a control that never appears
// being a mystery and being a line of output.
func Build(name string, deps Deps) (core.Plugin, error) {
	mu.RLock()
	f, ok := factories[name]
	mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("plugin: unknown plugin %q (known: %v)", name, Names())
	}

	if deps.Contributions != nil {
		deps.Contributions = &owned{name: name, next: deps.Contributions}
	}

	return f(deps)
}

// Names returns every registered plugin name, sorted.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
