package plugin

import "context"

// Run is the capability of asking the harness to do something: a task in, and
// the answer out.
//
// An interface rather than a field on Deps, for the same reason Notify is: the
// thing that can run a task is the frontend, which is built out of the tree, so
// a plugin asks the tree for the capability and works with whatever provides
// it. Nothing able to run a task is a miss rather than an error, and a plugin
// that finds nothing has simply nothing to ask.
//
// It exists so that starting a task is one thing the harness does, rather than
// something each interface assembles for itself out of a provider, a model and
// a loop. When the composer moves out of the web plugin, it will be the first
// thing to ask through here.
type Run interface {
	// Do drives one task to completion and returns the answer. It blocks until
	// the task is done.
	//
	// onStep, if set, is called with each tool call as it happens — the tool's
	// name and its raw arguments — so a caller can show progress rather than
	// waiting in silence. One task at a time: the agent holds a conversation,
	// and two at once would interleave into nonsense.
	Do(ctx context.Context, task string, onStep func(tool, args string)) (string, error)
}
