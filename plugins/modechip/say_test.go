package modechip

import (
	"testing"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
)

// sayer is a plugin that also listens: what a frontend looks like from a
// plugin's side, and all it has to be.
type sayer struct{ said []string }

func (s *sayer) Name() string                 { return "sayer" }
func (s *sayer) Setup() (func() error, error) { return func() error { return nil }, nil }
func (s *sayer) Say(text string)              { s.said = append(s.said, text) }

// speakers is a tree holding one such plugin.
type speakers struct{ s *sayer }

func (t *speakers) Mounted() []string   { return []string{"sayer"} }
func (t *speakers) Available() []string { return nil }
func (t *speakers) Get(name string) (core.Plugin, bool) {
	if name == "sayer" {
		return t.s, true
	}
	return nil, false
}
func (t *speakers) Mount(string) error   { return nil }
func (t *speakers) Unmount(string) error { return nil }

// switcher stands in for the modes seam.
type switcher struct{ mode string }

func (m *switcher) All() []plugin.Mode {
	return []plugin.Mode{
		{Name: "standard", Node: "standard"},
		{Name: "creator", Node: "creator"},
	}
}
func (m *switcher) Current() plugin.Mode  { return plugin.Mode{Name: m.mode, Node: m.mode} }
func (m *switcher) Set(name string) error { m.mode = name; return nil }

// catcher collects what the plugin offers.
type catcher struct{ items []plugin.Contribution }

func (c *catcher) Add(x plugin.Contribution)  { c.items = append(c.items, x) }
func (c *catcher) All() []plugin.Contribution { return c.items }

// Changing the mode says so. A setting that changes in silence is a setting
// people press twice to be sure it worked.
func TestChangingTheModeSaysSo(t *testing.T) {
	sw := &switcher{mode: "standard"}
	out := &catcher{}
	listener := &sayer{}

	p, err := plugin.Build("modechip", plugin.Deps{
		Modes:         sw,
		Contributions: out,
		Plugins:       &speakers{s: listener},
	})
	if err != nil {
		t.Fatalf("could not build the plugin: %v", err)
	}
	if _, err := p.Setup(); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if len(out.items) != 1 {
		t.Fatalf("offered %d contributions, want 1", len(out.items))
	}
	if out.items[0].Set == nil {
		t.Fatal("the selector cannot be changed")
	}

	if err := out.items[0].Set("creator"); err != nil {
		t.Fatalf("changing it failed: %v", err)
	}
	if sw.mode != "creator" {
		t.Errorf("the modes ended up on %q, want creator", sw.mode)
	}
	if len(listener.said) != 1 || listener.said[0] != "mode: creator" {
		t.Errorf("what was said = %v, want [mode: creator]", listener.said)
	}
}

// Nothing listening is not a failure: the switch happens, and the plugin keeps
// quiet because there is nobody to say it to.
func TestSayingNothingWhenNothingListens(t *testing.T) {
	sw := &switcher{mode: "standard"}
	out := &catcher{}

	p, err := plugin.Build("modechip", plugin.Deps{Modes: sw, Contributions: out})
	if err != nil {
		t.Fatalf("could not build the plugin: %v", err)
	}
	if _, err := p.Setup(); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if err := out.items[0].Set("creator"); err != nil {
		t.Fatalf("changing it with nothing listening failed: %v", err)
	}
	if sw.mode != "creator" {
		t.Errorf("the modes ended up on %q, want creator", sw.mode)
	}
}
