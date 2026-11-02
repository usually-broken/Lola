package plugin

import "github.com/usually-broken/lola/core/llm"

// Kind is a wire format an endpoint can speak.
//
// An endpoint is a url, a key and the shape of the conversation it expects.
// OpenAI's shape is the common one; it is not the only one, and nothing about a
// harness should assume it. A plugin that implements this says "I speak this
// shape", and an entry in the config names it.
type Kind interface {
	// llm.Adapter builds a provider for one endpoint in this plugin's shape.
	llm.Adapter

	// Kind is the name a config entry uses to ask for this shape: "openai",
	// "anthropic", whatever the shape is called.
	Kind() string
}

// Adapters is the set of shapes that are mounted, so a router can find the one an
// endpoint asked for rather than assuming.
//
// Nil on Deps means nothing is offering any shape, which is why a harness with no
// model adapter mounted cannot make a call at all — and says so, rather than
// failing somewhere deeper.
type Adapters interface {
	// Add offers a shape. Offering the same name twice replaces the earlier one.
	Add(kind Kind)

	// For answers with the shape of this name, if one is mounted.
	For(kind string) (llm.Adapter, bool)

	// Kinds lists what is mounted, for a screen or an error message.
	Kinds() []string
}
