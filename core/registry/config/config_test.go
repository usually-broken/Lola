package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/tools"
)

func TestParseReadsPluginList(t *testing.T) {
	cfg, err := Parse([]byte(`
# the tree
plugins:
  - greet
  - read_file
  - write_file
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := []string{"greet", "read_file", "write_file"}; !reflect.DeepEqual(cfg.Plugins, want) {
		t.Fatalf("plugins = %v, want %v", cfg.Plugins, want)
	}
}

func TestParseHandlesQuotesAndComments(t *testing.T) {
	cfg, err := Parse([]byte("plugins:\n  - \"greet\"  # the example\n  - 'read_file'\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := []string{"greet", "read_file"}; !reflect.DeepEqual(cfg.Plugins, want) {
		t.Fatalf("plugins = %v, want %v", cfg.Plugins, want)
	}
}

func TestParseRejectsListItemOutsidePlugins(t *testing.T) {
	_, err := Parse([]byte("  - greet\n"))
	if err == nil {
		t.Fatal("expected an error for a list item outside plugins:")
	}
}

func TestParseRejectsUnknownDirective(t *testing.T) {
	_, err := Parse([]byte("nonsense: true\n"))
	if err == nil {
		t.Fatal("expected an error for an unknown directive")
	}
}

func TestParseEmptyConfig(t *testing.T) {
	cfg, err := Parse([]byte("# nothing here\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Plugins) != 0 {
		t.Fatalf("plugins = %v, want empty", cfg.Plugins)
	}
}

type fakePlugin struct{ n string }

func (f *fakePlugin) Name() string                 { return f.n }
func (f *fakePlugin) Setup() (func() error, error) { return func() error { return nil }, nil }

func TestBuildMountsInConfigOrder(t *testing.T) {
	plugin.Register("cfg-a", func(plugin.Deps) (core.Plugin, error) { return &fakePlugin{n: "cfg-a"}, nil })
	plugin.Register("cfg-b", func(plugin.Deps) (core.Plugin, error) { return &fakePlugin{n: "cfg-b"}, nil })

	cfg := &Config{Plugins: []string{"cfg-b", "cfg-a"}}
	tree, err := cfg.Build(plugin.Deps{Tools: tools.New()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer tree.Unmount()

	if want := []string{"cfg-b", "cfg-a"}; !reflect.DeepEqual(tree.Names(), want) {
		t.Fatalf("names = %v, want %v", tree.Names(), want)
	}
}

func TestBuildUnknownPluginLeavesNothingMounted(t *testing.T) {
	plugin.Register("cfg-good", func(plugin.Deps) (core.Plugin, error) { return &fakePlugin{n: "cfg-good"}, nil })

	cfg := &Config{Plugins: []string{"cfg-good", "cfg-missing"}}
	_, err := cfg.Build(plugin.Deps{Tools: tools.New()})
	if err == nil {
		t.Fatal("expected an error for an unknown plugin")
	}
	if !strings.Contains(err.Error(), "cfg-missing") {
		t.Fatalf("error should name the missing plugin, got: %v", err)
	}
}

func TestBuildEmptyConfigMountsNothing(t *testing.T) {
	cfg := &Config{}
	tree, err := cfg.Build(plugin.Deps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(tree.Names()) != 0 {
		t.Fatalf("names = %v, want empty", tree.Names())
	}
}
