// Package promptaftertool contributes one instruction: after a tool returns, do not
// narrate what the person just watched.
//
// This is the rule that was hiding inside a plugin called "brevity", alongside a line
// telling the model to be short. The short line is gone: it was making replies clipped
// rather than making them readable. What remains is the specific habit — repeating back
// the step that just appeared on screen.
package promptaftertool

import (
	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/prompt"
)

// Order sits just after the pre-tool rule, and belongs beside it: they are the two
// halves of the same habit.
const Order = 12

const text = "After a tool returns, do not summarise what the person just watched happen. Say nothing, or add something they do not already have."

func init() {
	plugin.Register("promptaftertool", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{registry: d.Prompt}, nil
	})
}

type Plugin struct {
	registry *prompt.Registry
}

func (p *Plugin) Name() string { return "promptaftertool" }

func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindPrompt,
		What: "Don't repeat what happened after a tool: say nothing, or say something new.",
	}
}

func (p *Plugin) Setup() (func() error, error) {
	if p.registry == nil {
		return func() error { return nil }, nil
	}
	if err := p.registry.Add(prompt.Part{
		Name:  "after-a-tool",
		Order: Order,
		Text:  text,
		About: "No narrating the step that just appeared on screen.",
	}); err != nil {
		return nil, err
	}
	return func() error {
		p.registry.Remove("after-a-tool")
		return nil
	}, nil
}
