package openai

import "github.com/usually-broken/lola/core/llm"

// ProviderFor returns a provider for one endpoint.
//
// This is the half of the seam that lets the tree hand out model adapters
// without anything outside this package naming this package. The binary asks
// the tree for whatever can build a provider, and gets this.
func (p *Plugin) ProviderFor(baseURL, apiKey string) llm.Provider {
	return New(Options{BaseURL: baseURL, APIKey: apiKey})
}

// Assert at compile time that this adapter satisfies the seam.
var _ llm.Adapter = (*Plugin)(nil)
