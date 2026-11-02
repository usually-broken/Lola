package creator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/tools"
)

// probeTree is the plugin tree as the mode sees it.
type probeTree struct{ mounted []string }

func (t *probeTree) Mounted() []string         { return append([]string(nil), t.mounted...) }
func (t *probeTree) Available() []string       { return []string{"creator", "greet", "standard"} }
func (t *probeTree) Mount(name string) error   { t.mounted = append(t.mounted, name); return nil }
func (t *probeTree) Unmount(name string) error { return nil }

// Get hands back nothing, because this double counts names without holding any
// plugins. That is the honest answer rather than a pretend plugin.
func (t *probeTree) Get(name string) (core.Plugin, bool) { return nil, false }

var _ plugin.Plugins = (*probeTree)(nil)

func newPlugin(t *testing.T) (*Plugin, *tools.Registry, *probeTree) {
	t.Helper()

	registry := tools.New()
	tree := &probeTree{}
	p := New(registry, tree)
	if _, err := p.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	return p, registry, tree
}

// The capability is what the mode puts in the registry, so if the teardown does
// not take the tools away again, unmounting the mode leaves it able to act.
func TestTheModeAddsItsToolsAndTakesThemAwayAgain(t *testing.T) {
	registry := tools.New()
	p := New(registry, &probeTree{})

	teardown, err := p.Setup()
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	for _, want := range []string{"list_plugins", "mount_plugin"} {
		if _, ok := registry.Get(want); !ok {
			t.Fatalf("%s is not registered after setup, so the mode adds nothing", want)
		}
	}

	if err := teardown(); err != nil {
		t.Fatalf("teardown: %v", err)
	}
	for _, gone := range []string{"list_plugins", "mount_plugin"} {
		if _, ok := registry.Get(gone); ok {
			t.Fatalf("%s survived the teardown, so unmounting the mode leaves it able to act", gone)
		}
	}
}

// Mounting the same mode twice must not quietly stack two copies of its tools,
// because then the teardown would remove one and leave the other behind.
func TestTheModeCannotBeMountedTwice(t *testing.T) {
	registry := tools.New()

	if _, err := New(registry, &probeTree{}).Setup(); err != nil {
		t.Fatalf("first Setup: %v", err)
	}
	if _, err := New(registry, &probeTree{}).Setup(); err == nil {
		t.Fatal("the mode mounted twice and stacked its tools")
	}
}

func TestMountPluginMountsByNameAndRefusesAnEmptyName(t *testing.T) {
	_, registry, tree := newPlugin(t)

	tool, ok := registry.Get("mount_plugin")
	if !ok {
		t.Fatal("mount_plugin is missing")
	}

	if _, err := tool.Run(context.Background(), json.RawMessage(`{"name":"greet"}`)); err != nil {
		t.Fatalf("mounting greet: %v", err)
	}
	if len(tree.mounted) != 1 || tree.mounted[0] != "greet" {
		t.Fatalf("the tree did not get the plugin: %v", tree.mounted)
	}

	if _, err := tool.Run(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("mount_plugin accepted an empty name")
	}
}

func TestListPluginsReportsTheTree(t *testing.T) {
	_, registry, _ := newPlugin(t)

	list, ok := registry.Get("list_plugins")
	if !ok {
		t.Fatal("list_plugins is missing")
	}

	out, err := list.Run(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list_plugins: %v", err)
	}
	for _, want := range []string{"mounted:", "could be mounted:", "greet"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list_plugins did not report %q: %q", want, out)
		}
	}
}
