package creator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
)

// fakeTree is the smallest thing that answers like a tree.
type fakeTree struct {
	mounted   []string
	available []string
}

func (f *fakeTree) Mounted() []string              { return f.mounted }
func (f *fakeTree) Available() []string            { return f.available }
func (f *fakeTree) Get(string) (core.Plugin, bool) { return nil, false }
func (f *fakeTree) Mount(string) error             { return nil }
func (f *fakeTree) Unmount(string) error           { return nil }

var _ plugin.Plugins = (*fakeTree)(nil)

// The tool the model could not get an answer out of. If this passes, the fault is
// in the loop rather than in the tool.
func TestListPluginsReportsBothLists(t *testing.T) {
	tool := &listPlugins{tree: &fakeTree{
		mounted:   []string{"standard", "agent_loop"},
		available: []string{"creator", "tui"},
	}}

	out, err := tool.Run(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list_plugins returned an error: %v", err)
	}

	for _, want := range []string{"standard", "agent_loop", "creator", "tui"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output: %q", want, out)
		}
	}
}

func TestListPluginsWithoutATreeSaysSo(t *testing.T) {
	tool := &listPlugins{}

	out, err := tool.Run(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatalf("expected an error with no tree, got %q", out)
	}
}
