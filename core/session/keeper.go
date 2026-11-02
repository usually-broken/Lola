package session

// Keeper hands out the one session store the harness uses.
//
// Sessions are files in a single folder, and four separate places used to build
// their own store to reach them: the agent loop, both frontends, and the command
// line. A keeper is how that becomes one thing. The plugin that owns the folder
// offers this, and everything else asks the tree for a keeper rather than
// constructing its own.
type Keeper interface {
	// Sessions returns the store to read and write sessions through.
	Sessions() *Store
}
