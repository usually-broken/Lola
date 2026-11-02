// Package promptmode contributes what the harness currently is, in words.
//
// A mode decides which tools exist, which the model already learns from the tool
// list. This says it plainly as well, because a model that is told "you are in
// standard mode" stops guessing from the inventory — and because it is the pattern
// voice will follow when it contributes "you are being spoken to".
package promptmode

import (
	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/prompt"
)

// Order sits after behaviour: how to work, then what the harness is.
const Order = 30

func init() {
	plugin.Register("promptmode", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{registry: d.Prompt, modes: d.Modes}, nil
	})
}

// Plugin contributes one part, and takes it back when unmounted.
type Plugin struct {
	registry *prompt.Registry
	modes    plugin.Modes
}

func (p *Plugin) Name() string { return "promptmode" }

func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindPrompt,
		What: "Says which mode the harness is in, in words, rather than leaving it to be guessed from the tools.",
	}
}

// ModeName is what to call the mode in the prompt. Kept small so the text is a
// part's business, not the seam's.
func ModeName(m plugin.Mode) string {
	if m.Name != "" {
		return m.Name
	}
	return "standard"
}

func (p *Plugin) Setup() (func() error, error) {
	if p.registry == nil {
		return func() error { return nil }, nil
	}
	if p.modes == nil {
		// No modes offered means no honest thing to say about them.
		return func() error { return nil }, nil
	}

	// Answered when the prompt is assembled, not now: during the build the tree
	// is still being assembled, and asking it which mode is mounted would look up
	// something that does not exist yet.
	err := p.registry.Add(prompt.Part{
		Name:  "mode",
		Order: Order,
		Live: func() string {
			return "The harness is running in " + ModeName(p.modes.Current()) + " mode."
		},
		About: "Which mode is in force, said plainly.",
	})
	if err != nil {
		return nil, err
	}
	return func() error {
		p.registry.Remove("mode")
		return nil
	}, nil
}
