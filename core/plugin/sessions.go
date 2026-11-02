package plugin

import "time"

// Sessions is the store conversations are kept in, as a plugin sees it.
//
// It is an interface on Deps rather than a field carrying the store itself, for
// the same reason as the rest: a plugin that lists conversations should depend
// on what a conversation looks like, not on the package that happens to keep
// them today.
//
// Nil means nothing is offering conversations, which a plugin should treat as
// "there are none" rather than as an error.
type Sessions interface {
	// List is every saved conversation, newest first.
	List() ([]SessionInfo, error)

	// Delete removes one by id.
	Delete(id string) error
}

// SessionInfo is one conversation as a list shows it: enough to draw a row,
// and nothing that would make a frontend depend on how sessions are stored.
type SessionInfo struct {
	ID       string
	Title    string
	Updated  time.Time
	Pinned   bool
	Archived bool
	Count    int
}
