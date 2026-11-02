package llmrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/llm"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/routes"
)

func init() {
	plugin.Register("llmrouter", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{routes: d.Routes}, nil
	})
}

// Plugin is where model calls go. The registry behind it was in the core of the
// harness until it turned out to be the thing people most want to change: which
// endpoints exist, and what shape they speak.
type Plugin struct {
	mu       sync.RWMutex
	reg      *Registry
	routes   *routes.Registry
	adapters plugin.Adapters
}

// ModelsPath answers with what the endpoint in force offers.
const ModelsPath = "/api/models"

// Name identifies this plugin.
func (p *Plugin) Name() string { return "llmrouter" }

// About says what this is, so the plugins screen can group it and say what it
// is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindModel,
		What: "Where model calls go: the endpoints set up, and which one is in force. Switching takes effect on the next call.",
	}
}

// Setup does nothing: what it needs arrives from the config once the tree is up.
func (p *Plugin) Setup() (func() error, error) {
	if p.routes == nil {
		return func() error { return nil }, nil
	}
	if err := p.routes.Handle(ModelsPath, p.listModels); err != nil {
		return nil, err
	}
	return func() error {
		p.routes.Remove(ModelsPath)
		return nil
	}, nil
}

// listModels reports what the endpoint in force offers, so a model can be chosen
// from a control instead of remembered and typed.
func (p *Plugin) listModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	list, err := p.Models(ctx)
	if err != nil {
		// Not fatal: the screen shows the model in use and says so.
		log.Printf("llmrouter: could not list models: %v", err)
		_, _ = w.Write([]byte("[]"))
		return
	}

	_ = json.NewEncoder(w).Encode(list)
}

// Seed builds the registry from what the config said and what a previous run
// saved.
func (p *Plugin) Seed(specs []plugin.ProviderSpec, saved []plugin.ProviderSpec, storePath string, adapters plugin.Adapters) error {
	entries, err := Entries(toSpecs(specs))
	if err != nil {
		return err
	}

	if adapters == nil {
		return fmt.Errorf("llmrouter: nothing in the tree offers a wire format")
	}

	// Every entry has to name a shape something can speak, and saying so at
	// startup beats failing on the first call.
	for _, e := range entries {
		shape := shapeOf(e.Kind)
		if _, ok := adapters.For(shape); !ok {
			return fmt.Errorf("llmrouter: %q speaks %q, and nothing mounted offers that — mounted: %v",
				e.Name, shape, adapters.Kinds())
		}
	}

	reg, err := New(func(e Entry, key string) llm.Provider {
		adapter, ok := adapters.For(shapeOf(e.Kind))
		if !ok {
			return nil
		}
		return adapter.ProviderFor(e.URL, key)
	}, entries...)
	if err != nil {
		return err
	}

	if storePath != "" {
		reg.Keep(NewStore(storePath, toSpecs(saved)))
	}

	p.mu.Lock()
	p.reg = reg
	p.adapters = adapters
	p.mu.Unlock()
	return nil
}

// List is what is set up, and which one is in force.
func (p *Plugin) List() []plugin.ProviderInfo {
	reg := p.registry()
	if reg == nil {
		return nil
	}

	entries := reg.All()
	current := reg.Current()
	out := make([]plugin.ProviderInfo, 0, len(entries))
	for _, e := range entries {
		out = append(out, plugin.ProviderInfo{
			Name:    e.Name,
			URL:     e.URL,
			Kind:    e.Kind,
			KeyFrom: e.KeyFrom,
			Current: e.Name == current,
		})
	}
	return out
}

// Add sets up an endpoint.
func (p *Plugin) Add(spec plugin.ProviderSpec) error {
	reg := p.registry()
	if reg == nil {
		return fmt.Errorf("llmrouter: nothing set up yet")
	}
	// A shape nothing can speak would be saved and then refuse to start the
	// harness next time, so it is refused here instead.
	if shape := spec.Kind; shape != "" {
		p.mu.RLock()
		adapters := p.adapters
		p.mu.RUnlock()

		if adapters == nil {
			return fmt.Errorf("llmrouter: nothing in the tree offers a wire format")
		}
		if _, ok := adapters.For(shape); !ok {
			return fmt.Errorf("nothing mounted speaks %q — mounted: %v", shape, adapters.Kinds())
		}
	}

	_, err := reg.Add(Spec{Name: spec.Name, URL: spec.URL, Key: spec.Key, Kind: spec.Kind})
	return err
}

// Remove takes one out.
func (p *Plugin) Remove(name string) error {
	reg := p.registry()
	if reg == nil {
		return fmt.Errorf("llmrouter: nothing set up yet")
	}
	return reg.Remove(name)
}

// Choose picks the one in force.
func (p *Plugin) Choose(name string) error {
	reg := p.registry()
	if reg == nil {
		return fmt.Errorf("llmrouter: nothing set up yet")
	}
	return reg.Set(name)
}

// Current is the name of the one in force.
func (p *Plugin) Current() string {
	if reg := p.registry(); reg != nil {
		return reg.Current()
	}
	return ""
}

// Complete answers a request through whichever endpoint is in force.
func (p *Plugin) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	reg := p.registry()
	if reg == nil {
		return llm.Response{}, fmt.Errorf("llmrouter: nothing set up yet")
	}
	return reg.Complete(ctx, req)
}

// Models asks the endpoint in force what it offers.
func (p *Plugin) Models(ctx context.Context) ([]string, error) {
	reg := p.registry()
	if reg == nil {
		return nil, fmt.Errorf("llmrouter: nothing set up yet")
	}
	return reg.Models(ctx)
}

func (p *Plugin) registry() *Registry {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.reg
}

func toSpecs(in []plugin.ProviderSpec) []Spec {
	out := make([]Spec, 0, len(in))
	for _, s := range in {
		out = append(out, Spec{Name: s.Name, URL: s.URL, Key: s.Key, Kind: s.Kind})
	}
	return out
}

// Assert at compile time that this plugin is the router and a plugin.
var (
	_ core.Plugin   = (*Plugin)(nil)
	_ plugin.Router = (*Plugin)(nil)
)

// shapeOf is the wire format an entry asks for, with the empty case meaning the
// one every config written before there was a choice meant.
func shapeOf(kind string) string {
	if kind == "" {
		return "openai"
	}
	return kind
}

// Stream answers through whichever endpoint is in force, handing text back as it
// arrives when that endpoint can stream.
//
// Without this the loop never sees a stream: it asks the router, and a router that
// only knows Complete makes every provider beneath it look like one that cannot
// stream. A provider that genuinely cannot is handled by sending its whole answer
// through in one piece, which is what the caller would have got anyway.
func (p *Plugin) Stream(ctx context.Context, req llm.Request, onText func(string) error) (llm.Response, error) {
	reg := p.registry()
	if reg == nil {
		return llm.Response{}, fmt.Errorf("llmrouter: nothing set up yet")
	}

	provider, err := reg.provider()
	if err != nil {
		return llm.Response{}, err
	}

	if streamer, ok := provider.(llm.Streamer); ok {
		return streamer.Stream(ctx, req, onText)
	}

	resp, err := provider.Complete(ctx, req)
	if err != nil {
		return resp, err
	}
	if onText != nil && resp.Content != "" {
		if err := onText(resp.Content); err != nil {
			return resp, err
		}
	}
	return resp, nil
}

var _ llm.Streamer = (*Plugin)(nil)
