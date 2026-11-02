// Package promptpretool contributes one instruction: say a line before you act.
//
// One plugin, one rule. That is the whole design: a rule nobody can switch is a rule
// nobody can disagree with, and a rule bundled with four others cannot be judged.
package promptpretool

import (
	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/prompt"
)

// Order sits after persona and before the rest of the rules.
const Order = 10

const text = "Before you call a tool, say a short line in your own words about what you are about to do and why. Not a heading, not a plan — a sentence, the way you would say it aloud. Then make the call."

func init() {
	plugin.Register("promptpretool", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{registry: d.Prompt}, nil
	})
}

type Plugin struct {
	registry *prompt.Registry
}

func (p *Plugin) Name() string { return "promptpretool" }

func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindPrompt,
		What: "Say what you are about to do, in a sentence, before you call a tool.",
	}
}

func (p *Plugin) Setup() (func() error, error) {
	if p.registry == nil {
		return func() error { return nil }, nil
	}
	if err := p.registry.Add(prompt.Part{
		Name:  "before-a-tool",
		Order: Order,
		Text:  text,
		About: "A line before a call, spoken like a sentence.",
	}); err != nil {
		return nil, err
	}
	return func() error {
		p.registry.Remove("before-a-tool")
		return nil
	}, nil
}
