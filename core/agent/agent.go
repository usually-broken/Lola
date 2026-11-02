// Package agent is the agent seam. An agent takes a task and drives it to
// completion against a model and a set of tools.
//
// The seam is deliberately thin: the default loop lives in a plugin
// (plugins/agentloop), so a different loop can replace it. A root has to exist
// for the tree to plug into; this interface is that root, not the loop itself.
package agent

import (
	"context"
	"errors"

	"github.com/usually-broken/lola/core/llm"
)

// ErrMaxSteps is returned when a task runs longer than the step budget without
// the model finishing. It stops a looping model from running forever.
var ErrMaxSteps = errors.New("agent: reached the maximum number of steps")

// Result is what a run produced.
type Result struct {
	// Content is the model's final answer, with no tool call attached.
	Content string

	// Messages is the entire conversation, including every tool call and
	// result, in order. In a continuing conversation this is the whole
	// history, not just this run's exchanges.
	Messages []llm.Message

	// Steps is how many model calls this run took.
	Steps int
}

// Agent drives a task to completion. It is given the model adapter for the run,
// because the adapter is itself a plugin mounted elsewhere in the tree.
type Agent interface {
	// Run drives one task to completion against the given model adapter.
	//
	// On a conversation-carrying agent, the task is appended to the existing
	// history, so a follow-up sees what came before.
	Run(ctx context.Context, provider llm.Provider, model string, task string) (Result, error)

	// History returns the conversation so far, so a frontend can render it.
	History() []llm.Message

	// Reset starts a new conversation, discarding the history.
	Reset()

	// Resume loads a saved session by id, replacing the current conversation.
	// A loop with no session store returns an error.
	Resume(id string) error
}

// StepFunc is told about one step of a run: the model's reply, which either
// calls tools or ends the run.
type StepFunc func(step int, msg llm.Message)

// Watcher is an agent that can report its steps as it works, so a caller can
// show the loop running rather than only its answer.
//
// Deliberately optional. Watching is not part of driving a task, so an agent
// that cannot report its steps is still a perfectly good agent — a caller that
// wants to watch says so by asking for this interface, and takes no for an
// answer.
type Watcher interface {
	// Watch registers a function to be called with each step's assistant
	// message, including the steps that call no tools.
	Watch(onStep StepFunc)
}

// Conversation carries the messages of a session across runs, so a follow-up
// task is a continuation rather than a fresh start.
//
// It is a plain value, not a plugin: an agent owns one, and other agents with
// the same need use the same type. Keeping it here means "what a conversation
// is" has one definition rather than one per loop.
type Conversation struct {
	messages []llm.Message
}

// Messages returns the conversation's messages in order.
func (c *Conversation) Messages() []llm.Message { return c.messages }

// Append adds messages to the conversation.
func (c *Conversation) Append(msgs ...llm.Message) { c.messages = append(c.messages, msgs...) }

// Reset clears the conversation.
func (c *Conversation) Reset() { c.messages = nil }

// Len returns how many messages the conversation holds.
func (c *Conversation) Len() int { return len(c.messages) }
