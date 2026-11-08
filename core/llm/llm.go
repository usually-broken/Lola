// Package llm is the model adapter seam. Everything that talks to a model
// talks to the Provider interface below instead, so switching providers is a
// configuration change rather than a code change.
package llm

import (
	"context"
	"encoding/json"
	"time"
)

// Role identifies who produced a message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one entry in the conversation sent to a model.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`

	// ToolCalls is set on an assistant message when the model asked for one
	// or more tools to run.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// ToolCallID is set on a tool message to say which call it answers.
	ToolCallID string `json:"tool_call_id,omitempty"`

	// Timing rides with the assistant message that it describes, so how long the
	// reply took is stored beside the reply rather than in a separate record that
	// would have to be kept in step with it.
	Timing *Timing `json:"timing,omitempty"`
}

// Timing is how long a reply took to start, and how fast it then arrived.
type Timing struct {
	// FirstWordMS is milliseconds from sending the request to the first piece of
	// text. Zero when the provider does not stream, because then there is no
	// first piece to measure.
	FirstWordMS int64 `json:"firstWordMs"`

	// TokensPerSecond is how quickly the rest arrived, once it had started,
	// counted in tokens rather than characters because that is the unit anyone
	// talking about model speed means.
	TokensPerSecond float64 `json:"tokensPerSecond"`

	// Tokens is how many the provider reported for this reply, or zero when it
	// reported none — a model's own count is the only honest one.
	Tokens int `json:"tokens,omitempty"`

	// Chunks is how many pieces the reply arrived in. A reply that came in one
	// piece was never seen decoding, so its rate cannot be measured.
	Chunks int `json:"chunks,omitempty"`

	// Model is which model produced the reply, so speeds can be compared rather
	// than averaged into one meaningless number.
	Model string `json:"model,omitempty"`

	// At is when the reply was made, which is the only way "the last 24 hours"
	// can mean anything later.
	At time.Time `json:"at,omitempty"`
}

// ToolCall is a model's request to run one tool.
type ToolCall struct {
	// ID identifies the call so its result can be matched back to it.
	ID string `json:"id"`

	// Name is the tool the model wants to run.
	Name string `json:"name"`

	// Arguments is the raw JSON the model supplied for the tool.
	Arguments json.RawMessage `json:"arguments"`
}

// ToolDef describes one tool to the model so it knows the tool exists and how
// to call it.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

// Request is one call to a model.
type Request struct {
	Model       string
	Messages    []Message
	Tools       []ToolDef
	Temperature *float64
}

// Response is what a model returned.
type Response struct {
	// Content is the model's text, which may be empty when it called tools.
	Content string

	// ToolCalls is what the model wants run, if anything.
	ToolCalls []ToolCall

	// Tokens is how many the provider says the answer cost, when it says.
	Tokens int
}

// Provider is the seam. A provider answers a Request with a Response. Every
// concrete model adapter — Ollama today, a hosted API tomorrow — implements
// exactly this, and consumers never know which one they got.
type Provider interface {
	// Complete sends the conversation to the model and returns its reply.
	Complete(ctx context.Context, req Request) (Response, error)

	// Name identifies the provider in logs and configuration.
	Name() string
}

// Streamer is a provider that can answer in pieces.
//
// It exists as a second interface rather than a method on Provider so a provider
// that cannot stream is still a provider. Callers that want live text check for
// it and fall back to one final answer, which is exactly what a model that
// cannot stream should look like from the other side.
type Streamer interface {
	Provider

	// Stream sends the conversation and calls onText with each piece of text as
	// it arrives, returning the finished Response once the model stops.
	//
	// Tool calls arrive as fragments — an argument string split across chunks —
	// so they are assembled here and returned whole: a caller should never have
	// to know a call was once in pieces.
	Stream(ctx context.Context, req Request, onText func(delta string) error) (Response, error)
}
