package plugin

// Keeper is the capability of remembering a choice between runs.
//
// Deliberately a key-value store with scopes rather than a settings object, so
// the core does not have to know what any particular choice means. A model and a
// mode are kept the same way as something a plugin invented yesterday, and the
// thing that keeps them never learns which is which.
//
// Scope is "global" for a choice that applies to new sessions, and
// "session:<id>" for one that belongs to a single conversation. A miss is not an
// error: nothing remembered means the caller's own default, which is the safe
// thing to fall back to.
type Keeper interface {
	// Get answers with what was kept, if anything was.
	Get(scope, key string) (string, bool)

	// Set keeps a value. An error means it was not kept, and a caller that cares
	// can say so; a caller that does not can carry on with the value in memory.
	Set(scope, key, value string) error
}
