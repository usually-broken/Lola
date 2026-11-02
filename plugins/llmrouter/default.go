package llmrouter

// DefaultURL is where calls go when nothing says otherwise: the local Ollama
// server. It lives here rather than in a model adapter because this seam owns
// where calls go, and a harness pointed at nothing should still work.
const DefaultURL = "http://127.0.0.1:11434/v1"
