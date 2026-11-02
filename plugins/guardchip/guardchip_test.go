package guardchip

import (
	"strings"
	"testing"

	"github.com/usually-broken/lola/core/plugin"
)

// consent is a stand-in for the guard's rules, so the plugin can be tested
// without a guard behind it.
type consent struct{ mode string }

func (c *consent) Mode() string           { return c.mode }
func (c *consent) Modes() []string        { return []string{"workspace", "ask", "open"} }
func (c *consent) SetMode(m string) error { c.mode = m; return nil }

// collected is a stand-in for whatever is collecting contributions.
type collected struct{ items []plugin.Contribution }

func (c *collected) Add(x plugin.Contribution)  { c.items = append(c.items, x) }
func (c *collected) All() []plugin.Contribution { return c.items }

// The selector says what the guard is doing, offers what the guard allows, and
// changes through the guard — and it lands inside the card, which is where the
// approval control has always been.
func TestTheSelectorComesFromTheGuard(t *testing.T) {
	rules := &consent{mode: "workspace"}
	out := &collected{}

	p, err := plugin.Build("guardchip", plugin.Deps{Consent: rules, Contributions: out})
	if err != nil {
		t.Fatalf("could not build the plugin: %v", err)
	}
	if _, err := p.Setup(); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if len(out.items) != 1 {
		t.Fatalf("offered %d contributions, want 1", len(out.items))
	}
	c := out.items[0]

	if c.Place != PlaceComposerTools {
		t.Errorf("offered for place %q, want %q", c.Place, PlaceComposerTools)
	}
	if c.Kind != plugin.KindSelect {
		t.Errorf("offered as %q, want a selector", c.Kind)
	}
	if c.Owner != "guardchip" {
		t.Errorf("offered as %q, want the harness to have stamped it guardchip", c.Owner)
	}

	value, options := c.Current()
	if value != "workspace" {
		t.Errorf("reported %q, want workspace", value)
	}
	if len(options) != 3 {
		t.Errorf("offered %d choices, want 3", len(options))
	}

	if c.Set == nil {
		t.Fatal("nothing to change it with")
	}
	if err := c.Set("ask"); err != nil {
		t.Fatalf("changing it failed: %v", err)
	}
	if rules.mode != "ask" {
		t.Fatalf("the guard ended up on %q, want ask", rules.mode)
	}
}

// The selector says what the setting covers, because the natural reading of
// "approval" is that it covers everything — and now, in the mode that covers
// everything, it does.
func TestTheSelectorSaysWhatItCovers(t *testing.T) {
	rules := &consent{mode: "workspace"}
	out := &collected{}

	p, err := plugin.Build("guardchip", plugin.Deps{Consent: rules, Contributions: out})
	if err != nil {
		t.Fatalf("could not build the plugin: %v", err)
	}
	if _, err := p.Setup(); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	note := out.items[0].Note
	if note == "" {
		t.Fatal("the selector carries no note, so the person is left to guess what it covers")
	}
	if !strings.Contains(note, "every tool call") {
		t.Errorf("the note does not say what ask mode covers: %q", note)
	}
	if !strings.Contains(note, "every mode") {
		t.Errorf("the note does not say the wall holds everywhere: %q", note)
	}
}

// With nothing offering consent, it puts nothing on screen. A control that
// cannot work is worse than no control.
func TestWithNothingOfferingConsentItOffersNothing(t *testing.T) {
	out := &collected{}

	p, err := plugin.Build("guardchip", plugin.Deps{Contributions: out})
	if err != nil {
		t.Fatalf("could not build the plugin: %v", err)
	}
	if _, err := p.Setup(); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if len(out.items) != 0 {
		t.Fatalf("offered %d contributions with no consent behind it, want none", len(out.items))
	}
}
