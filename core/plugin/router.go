package plugin

import "github.com/usually-broken/lola/core/llm"

// Router is where model calls go, as a plugin sees it.
//
// It is an interface on the seam, not a package, so which endpoints exist and
// how any one of them is spoken to is a plugin's business rather than the
// harness's. A plugin that offers this is the one the loop asks.
//
// Keys never pass through here: a spec names the environment variable its key
// comes from, and the key is resolved at the moment of a call.
type Router interface {
	// llm.Provider is the half the loop uses: Complete and Models.
	llm.Provider

	// Seed is how the harness hands over what the config file said at startup,
	// plus anything saved by a previous run. Called once, before the first call.
	Seed(specs []ProviderSpec, saved []ProviderSpec, storePath string, adapters Adapters) error

	// List is what is set up, and which one is in force.
	List() []ProviderInfo

	// Add sets up an endpoint. A spec with no name or url is refused.
	Add(spec ProviderSpec) error

	// Remove takes one out. The one in force cannot be removed.
	Remove(name string) error

	// Choose picks the one in force, for the next call rather than the next
	// start.
	Choose(name string) error

	// Current is the name of the one in force.
	Current() string
}

// ProviderSpec is one endpoint, as configuration and as what a button adds.
type ProviderSpec struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Key  string `json:"key"`

	// Kind is the wire format this endpoint speaks. Empty means "openai", which
	// is what every config written before there was a choice means.
	Kind string `json:"kind,omitempty"`
}

// ProviderInfo is one endpoint as a screen shows it: enough to draw a row, and
// never the key itself.
type ProviderInfo struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Kind    string `json:"kind,omitempty"`
	KeyFrom string `json:"keyFrom,omitempty"`
	Current bool   `json:"current"`
}
