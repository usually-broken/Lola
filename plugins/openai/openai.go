// Package openai implements llm.Provider for any OpenAI-compatible
// /chat/completions endpoint. Ollama's local server and hosted gateways such
// as OpenRouter speak the same shape, so pointing the harness at a different
// backend is a matter of changing BaseURL and APIKey.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/llm"
	"github.com/usually-broken/lola/core/plugin"
)

func init() {
	plugin.Register("openai", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{provider: New(Options{}), adapters: d.Adapters}, nil
	})
}

// Plugin wraps the provider so it can be mounted like any other plugin. The
// provider itself is what consumers use; the plugin's job is to exist in the
// tree and hand out that provider.
type Plugin struct {
	adapters plugin.Adapters
	provider *Provider
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "openai" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindModel,
		What: "Where thinking happens: an OpenAI-compatible endpoint.",
	}
}

// Setup mounts the plugin. The provider needs nothing set up.
func (p *Plugin) Setup() (func() error, error) {
	// This plugin is a wire format as well as a provider: it says it speaks the
	// OpenAI shape, so an endpoint can ask for it by name.
	if p.adapters != nil {
		p.adapters.Add(kind{})
	}
	return func() error { return nil }, nil
}

// kind is the OpenAI shape, as an adapter a router can find by name.
type kind struct{}

// Kind is the name a config entry uses to ask for this shape.
func (kind) Kind() string { return "openai" }

// ProviderFor builds a provider for one endpoint speaking this shape.
func (kind) ProviderFor(baseURL, apiKey string) llm.Provider {
	return New(Options{BaseURL: baseURL, APIKey: apiKey})
}

var _ plugin.Kind = kind{}

// Provider returns the model adapter this plugin contributes.
func (p *Plugin) Provider() llm.Provider { return p.provider }

// DefaultBaseURL is the local Ollama server. The providers seam owns this
// value, since it owns where calls go; the name stays here for callers who
// think in terms of this adapter.
// DefaultBaseURL is where a local Ollama listens.
const DefaultBaseURL = "http://127.0.0.1:11434/v1"

// Provider talks to an OpenAI-compatible chat completions endpoint.
type Provider struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// Options configures a Provider.
type Options struct {
	// BaseURL is the endpoint root, without /chat/completions. Defaults to
	// DefaultBaseURL when empty.
	BaseURL string

	// APIKey is sent as a bearer token when non-empty. Local servers usually
	// do not need one.
	APIKey string

	// HTTPClient overrides the default client. Useful in tests.
	HTTPClient *http.Client
}

// New returns a Provider for an OpenAI-compatible endpoint.
func New(opts Options) *Provider {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}

	return &Provider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  opts.APIKey,
		client:  client,
	}
}

// Name identifies this provider.
func (p *Provider) Name() string { return "openai" }

// --- wire types -----------------------------------------------------------

type wireFunction struct {
	Name string `json:"name"`

	// Arguments is the JSON the tool is called with, carried as a *string*: the
	// OpenAI shape defines it that way, and sending an object back is rejected
	// on the next step with a json unmarshal error.
	Arguments string `json:"arguments"`
}

type wireToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []wireMessage `json:"messages"`
	Tools       []wireTool    `json:"tools,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	Stream      bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message wireMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// --- conversion -----------------------------------------------------------

func toWireMessages(msgs []llm.Message) []wireMessage {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		wm := wireMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
				ID:       tc.ID,
				Type:     "function",
				Function: wireFunction{Name: tc.Name, Arguments: argumentString(tc.Arguments)},
			})
		}
		out = append(out, wm)
	}
	return out
}

func toWireTools(defs []llm.ToolDef) []wireTool {
	out := make([]wireTool, 0, len(defs))
	for _, d := range defs {
		var wt wireTool
		wt.Type = "function"
		wt.Function.Name = d.Name
		wt.Function.Description = d.Description
		wt.Function.Parameters = d.Schema
		out = append(out, wt)
	}
	return out
}

// --- the seam -------------------------------------------------------------

// Complete sends the conversation to the endpoint and returns the reply.
func (p *Provider) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	body, err := json.Marshal(chatRequest{
		Model:       req.Model,
		Messages:    toWireMessages(req.Messages),
		Tools:       toWireTools(req.Tools),
		Temperature: req.Temperature,
	})
	if err != nil {
		return llm.Response{}, fmt.Errorf("openai: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body),
	)
	if err != nil {
		return llm.Response{}, fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("openai: send request: %w", err)
	}
	defer httpResp.Body.Close()

	var parsed chatResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&parsed); err != nil {
		return llm.Response{}, fmt.Errorf("openai: decode response (status %s): %w", httpResp.Status, err)
	}

	if parsed.Error != nil {
		return llm.Response{}, fmt.Errorf("openai: %s", parsed.Error.Message)
	}
	if httpResp.StatusCode != http.StatusOK {
		return llm.Response{}, fmt.Errorf("openai: unexpected status %s", httpResp.Status)
	}
	if len(parsed.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("openai: response contained no choices")
	}

	msg := parsed.Choices[0].Message
	resp := llm.Response{Content: msg.Content}
	for _, tc := range msg.ToolCalls {
		// Coming in, the arguments are a string; they are decoded into the JSON
		// object the tool expects.
		args := tc.Function.Arguments
		if args == "" {
			args = "{}"
		}
		resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(args),
		})
	}
	return resp, nil
}

// modelsResponse is the shape both Ollama and hosted gateways use for a model
// list: an array under "data", each entry carrying an "id".
type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// Models lists what the endpoint offers, so a settings screen can offer a
// choice rather than making someone remember a name.
func (p *Provider) Models(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("openai: build models request: %w", err)
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: list models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: list models: unexpected status %s", resp.Status)
	}

	var parsed modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("openai: list models: %w", err)
	}

	out := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	sort.Strings(out)
	return out, nil
}

// argumentString is the arguments as the string the wire format wants. Empty or
// unreadable arguments become "{}", which is what every tool accepts.
func argumentString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}
