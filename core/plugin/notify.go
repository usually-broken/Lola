package plugin

// Notify is the capability of being told something by a plugin: an interface
// that is running and willing to show a line from somewhere else in the tree.
//
// It is an interface rather than a field on Deps for a reason that is not
// stylistic. The notifier *is* the frontend, and the frontend is built out of
// the tree, so a field could only ever be filled after the things that need it
// had already been built. So a plugin finds it through the tree, the way the
// agent loop finds the session store, and asks for the capability rather than
// for the name of whichever plugin happens to provide it.
//
// Nothing listening is a miss, not an error. A plugin that has something to say
// in a tree where nothing is drawing an interface has simply said nothing, and
// should carry on doing its work.
type Notify interface {
	// Say sends a line to the interface, for a person to read.
	//
	// This is narration, not the assistant speaking: what a plugin says about
	// its own work, in its own words. A frontend draws it the quiet way it draws
	// anything that is not the model talking.
	Say(text string)
}
