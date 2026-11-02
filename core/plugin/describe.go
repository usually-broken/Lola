package plugin

// A plugin can say what kind of thing it is, without the screen having to know.
//
// The list of plugins grew past the point where a flat alphabetical column
// told anyone anything: a keeper that stops conversations being saved sat next
// to a demo that says hello. Saying so belongs here, in the plugin, because the
// plugin is the only thing that knows.
//
// Describing is optional. A plugin that says nothing is still a plugin, and the
// screen will group it with the rest rather than refuse to draw it — a
// declaration that is required is a tax on every plugin written later.
type Described interface {
	About() About
}

// About is what a plugin says about itself.
type About struct {
	// Kind is what sort of thing this is, in one of the Kind constants below.
	// Anything unrecognised is shown as it is written.
	Kind string

	// What is one sentence, for a human reading the list, saying what this
	// plugin actually does. Not what it is called — what it is for.
	What string

	// Vital says this plugin should not be taken out of a running tree.
	// Setting it does not make the switch safe to ignore: the tree refuses the
	// request and says why, because a switch that is only greyed out is still a
	// switch that does nothing.
	Vital bool
}

// The kinds a plugin can say it is. They are strings rather than an enum so a
// plugin written later can introduce one without editing this file, and the
// screen shows whatever it does not recognise under its own heading.
const (
	// KindUI is something a person looks at or types into.
	KindUI = "ui"

	// KindTool is something the model can call.
	KindTool = "tool"

	// KindChip is a small piece of interface contributed to a screen. The word
	// already exists on the contribution seam; this file does not redeclare it,
	// so a plugin saying KindChip and a contribution placing itself at KindChip
	// mean the same thing.

	// KindKeeper is something that remembers, or owns where things are kept.
	KindKeeper = "keeper"

	// KindModel is where the thinking happens.
	KindModel = "model"

	// KindVoice is a piece of speaking or hearing: the mouth, the ear, or the mode
	// that puts them together. Its own kind because none of them are tools the model
	// can call — they are things the interface calls.
	KindVoice = "voice"

	// KindPrompt is something that contributes to what the model is told before
	// anything else. A kind of its own because prompt contributions are read as
	// one document, so they belong together rather than scattered by what else
	// they happen to do.
	KindPrompt = "prompt"

	// KindCore is the machinery a harness cannot do without.
	KindCore = "core"
)
