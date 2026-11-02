package plugin

// Contribution is something a plugin wants to appear on an interface.
//
// It is described rather than drawn, for the same reason a pane is: a browser
// renders a switch one way and a terminal renders it another, and neither of
// them needs to know what the other does. What a plugin says is "I am a
// selector, here is my label, here are my choices, here is where I want to
// live" — and the frontend decides how that looks.
//
// The kinds are deliberately few and ordinary. A new kind is a small addition
// here and a case in each frontend, not a new concept.
type Contribution struct {
	// Place is where it goes — "above-composer", "composer-tools", "settings".
	// Places belong to the frontend: it knows which ones exist and where they
	// are, and a contribution naming one it does not know is not rendered.
	Place string `json:"place"`

	// Kind is what it is: "chip", "toggle", "select", "text", "panel", or
	// "fragment".
	Kind string `json:"kind"`

	// Name identifies this contribution, so a frontend can tell two of them
	// apart and so a change can be sent back for the right one.
	Name string `json:"name"`

	// Owner is the plugin that offered it, stamped by the harness on the way
	// through rather than written here. A plugin naming itself is a plugin that
	// can lie or forget, and the moment it matters is when something needs
	// tracking down.
	Owner string `json:"owner,omitempty"`

	// Label is what a person reads.
	Label string `json:"label"`

	// Note, if set, is a line of help about the thing, for a frontend to show
	// where it shows help. It exists because the label cannot carry an
	// explanation, and a setting that does not say what it covers gets
	// misunderstood — which is how "ask before it runs" came to be read as
	// "ask before anything, including reading".
	Note string `json:"note,omitempty"`

	// Value is what it currently says or is set to.
	Value string `json:"value,omitempty"`

	// Options are the choices, for a selector.
	Options []string `json:"options,omitempty"`

	// Changeable says whether a frontend can change it. It is filled in here by
	// the registry, from whether the offering plugin gave it something to
	// change with, so a frontend can draw a control that can be pressed or a
	// label that cannot without asking and finding out.
	Changeable bool `json:"changeable,omitempty"`

	// Set, if set, applies a change to this contribution's value, with whatever
	// the person chose. A frontend that can change it calls this.
	//
	// It is Go-side only and never travels: the seam carries behaviour rather
	// than an address, so a plugin offering a control knows nothing about the
	// interface drawing it, and no plugin has to name another plugin's route.
	Set func(value string) error `json:"-"`

	// Current, if set, is asked for the value and the choices each time the
	// list is read, so an interface shows what is true now rather than what was
	// true when the plugin mounted. A mode that has just been switched should
	// not still be displaying the old one.
	//
	// It is Go-side only and never travels: the registry fills Value and
	// Options from it before anything is written out.
	Current func() (value string, options []string) `json:"-"`

	// Draw, if set, is asked for a fragment's markup each time the list is
	// read, for the same reason Current is asked: what a fragment says should
	// be true when it is drawn, not when it was offered.
	//
	// Markup is the one place a plugin says how something looks rather than
	// what it is, and it is allowed because a plugin is compiled into the
	// harness — this is the harness's own code, not a stranger's. It is also
	// why a frontend may decline to draw it: a terminal has nowhere to put
	// markup, and leaving the kind unrendered is the honest answer rather than
	// printing the tags.
	Draw func() string `json:"-"`
}

// The kinds, named so that a typo is a compile error in the offering plugin
// rather than a contribution that quietly never appears.
const (
	KindChip     = "chip"
	KindToggle   = "toggle"
	KindSelect   = "select"
	KindText     = "text"
	KindPanel    = "panel"
	KindFragment = "fragment"
)

// Contributions is where plugins offer things to the interface.
//
// One interface on Deps, like the rest: whatever draws the frontend depends on
// the shape, not on the package holding the list.
type Contributions interface {
	// Add offers a contribution. Called by a plugin during its setup.
	Add(c Contribution)

	// All lists what has been offered, in the order it was offered, which for
	// plugins is mount order.
	All() []Contribution
}
