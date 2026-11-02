package plugin

import (
	"errors"
	"strings"
	"testing"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/tools"
)

type noop struct{ name string }

func (n *noop) Name() string                 { return n.name }
func (n *noop) Setup() (func() error, error) { return func() error { return nil }, nil }

func TestBuildReturnsRegisteredPlugin(t *testing.T) {
	Register("test-noop", func(Deps) (core.Plugin, error) { return &noop{name: "test-noop"}, nil })

	p, err := Build("test-noop", Deps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if p.Name() != "test-noop" {
		t.Fatalf("name = %q, want test-noop", p.Name())
	}
}

func TestBuildUnknownPluginNamesTheKnownOnes(t *testing.T) {
	Register("test-known", func(Deps) (core.Plugin, error) { return &noop{name: "test-known"}, nil })

	_, err := Build("does-not-exist", Deps{})
	if err == nil {
		t.Fatal("expected an error for an unknown plugin")
	}
	if !strings.Contains(err.Error(), "does-not-exist") || !strings.Contains(err.Error(), "test-known") {
		t.Fatalf("error should name the missing and the known plugins, got: %v", err)
	}
}

func TestFactoryReceivesDeps(t *testing.T) {
	reg := tools.New()
	var got *tools.Registry

	Register("test-deps", func(d Deps) (core.Plugin, error) {
		got = d.Tools
		return &noop{name: "test-deps"}, nil
	})

	if _, err := Build("test-deps", Deps{Tools: reg}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got != reg {
		t.Fatal("factory did not receive the tool registry from Deps")
	}
}

func TestFactoryErrorPropagates(t *testing.T) {
	boom := errors.New("factory boom")
	Register("test-boom", func(Deps) (core.Plugin, error) { return nil, boom })

	_, err := Build("test-boom", Deps{})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestNamesIsSortedAndIncludesRegistered(t *testing.T) {
	Register("test-aaa", func(Deps) (core.Plugin, error) { return &noop{name: "test-aaa"}, nil })
	Register("test-zzz", func(Deps) (core.Plugin, error) { return &noop{name: "test-zzz"}, nil })

	names := Names()
	for _, want := range []string{"test-aaa", "test-noop", "test-zzz"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("Names() = %v, missing %q", names, want)
		}
	}
}

func TestRegisterPanicsOnDuplicate(t *testing.T) {
	Register("test-dup", func(Deps) (core.Plugin, error) { return &noop{name: "test-dup"}, nil })

	defer func() {
		if recover() == nil {
			t.Fatal("expected Register to panic on a duplicate name")
		}
	}()
	Register("test-dup", func(Deps) (core.Plugin, error) { return &noop{name: "test-dup"}, nil })
}
