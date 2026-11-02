package llm

// Adapter builds a provider for one endpoint.
//
// A model adapter plugin supplies this, which is what lets the harness hand out
// model adapters without the binary knowing which one is mounted — or that there
// is an OpenAI-compatible one at all. The seam is in two halves for that reason:
// Provider is what a call is made through, Adapter is what callers are handed.
type Adapter interface {
	// ProviderFor returns a provider for the given endpoint. An empty key means
	// the endpoint needs none, which is the usual case for a local server.
	ProviderFor(baseURL, apiKey string) Provider
}
