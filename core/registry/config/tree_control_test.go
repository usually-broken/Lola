package config

import (
	"testing"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
)

// These factories are registered once per test binary, because Register panics
// on a duplicate name. Each build records what it produced so a test can check
// the plugin was actually set up rather than just listed.

var (
	builtOne *probePlugin
	builtTwo *probePlugin
)

func init() {
	plugin.Register("probe_one", func(plugin.Deps) (core.Plugin, error) {
		builtOne = &probePlugin{name: "probe_one"}
		return builtOne, nil
	})
	plugin.Register("probe_two", func(plugin.Deps) (core.Plugin, error) {
		builtTwo = &probePlugin{name: "probe_two"}
		return builtTwo, nil
	})
}

// probePlugin records whether it was set up and torn down.
type probePlugin struct {
	name     string
	setUp    bool
	tornDown bool
}

func (p *probePlugin) Name() string { return p.name }

func (p *probePlugin) Setup() (func() error, error) {
	p.setUp = true
	return func() error {
		p.tornDown = true
		return nil
	}, nil
}

func TestMountOneAddsAPluginToARunningTree(t *testing.T) {
	tree := &Tree{}
	builtOne = nil

	if err := tree.MountOne(plugin.Deps{}, "probe_one"); err != nil {
		t.Fatalf("MountOne: %v", err)
	}

	names := tree.Names()
	if len(names) != 1 || names[0] != "probe_one" {
		t.Fatalf("want [probe_one], got %v", names)
	}
	if builtOne == nil || !builtOne.setUp {
		t.Fatal("the plugin was listed but never set up")
	}
}

func TestMountOneRefusesADuplicateAndLeavesTheTree(t *testing.T) {
	tree := &Tree{}
	if err := tree.MountOne(plugin.Deps{}, "probe_one"); err != nil {
		t.Fatalf("first mount: %v", err)
	}

	if err := tree.MountOne(plugin.Deps{}, "probe_one"); err == nil {
		t.Fatal("mounting the same plugin twice should be refused")
	}

	// The point of the behaviour: a duplicate must not disturb what is
	// already running. Build's job is to tear everything down; this one's is
	// to leave it alone.
	if names := tree.Names(); len(names) != 1 {
		t.Fatalf("the tree was disturbed: %v", names)
	}
}

func TestMountOneRefusesAnUnknownNameAndChangesNothing(t *testing.T) {
	tree := &Tree{}
	if err := tree.MountOne(plugin.Deps{}, "probe_one"); err != nil {
		t.Fatalf("mount: %v", err)
	}

	if err := tree.MountOne(plugin.Deps{}, "no_such_plugin"); err == nil {
		t.Fatal("an unknown plugin should be refused")
	}

	if names := tree.Names(); len(names) != 1 || names[0] != "probe_one" {
		t.Fatalf("the tree changed on a refused mount: %v", names)
	}
}

func TestUnmountOneTakesOutOneAndLeavesTheRest(t *testing.T) {
	tree := &Tree{}
	if err := tree.MountOne(plugin.Deps{}, "probe_one"); err != nil {
		t.Fatalf("mount one: %v", err)
	}
	if err := tree.MountOne(plugin.Deps{}, "probe_two"); err != nil {
		t.Fatalf("mount two: %v", err)
	}
	builtOne.tornDown = false

	if err := tree.UnmountOne("probe_one"); err != nil {
		t.Fatalf("UnmountOne: %v", err)
	}

	names := tree.Names()
	if len(names) != 1 || names[0] != "probe_two" {
		t.Fatalf("want [probe_two] left mounted, got %v", names)
	}
	if !builtOne.tornDown {
		t.Fatal("the plugin was removed from the list but never torn down")
	}
}

func TestUnmountOneRefusesANameThatIsNotMounted(t *testing.T) {
	tree := &Tree{}
	if err := tree.MountOne(plugin.Deps{}, "probe_one"); err != nil {
		t.Fatalf("mount: %v", err)
	}

	if err := tree.UnmountOne("probe_two"); err == nil {
		t.Fatal("unmounting something that is not mounted should be refused")
	}

	if names := tree.Names(); len(names) != 1 || names[0] != "probe_one" {
		t.Fatalf("the tree changed on a refused unmount: %v", names)
	}
}
