package modes

import (
	"errors"
	"strings"
	"testing"

	"github.com/usually-broken/lola/core/plugin"
)

var errProbe = errors.New("probe")

// probeTree remembers what is mounted, which is all a switch needs to be tested
// against. It can be told to fail one mount, to prove a failed switch is not a
// half-done one.
type probeTree struct {
	mounted   []string
	failMount string
}

func (t *probeTree) Mounted() []string { return append([]string(nil), t.mounted...) }

func (t *probeTree) Mount(name string) error {
	if name == t.failMount {
		return errProbe
	}
	for _, m := range t.mounted {
		if m == name {
			return errProbe
		}
	}
	t.mounted = append(t.mounted, name)
	return nil
}

func (t *probeTree) Unmount(name string) error {
	for i, m := range t.mounted {
		if m == name {
			t.mounted = append(t.mounted[:i], t.mounted[i+1:]...)
			return nil
		}
	}
	return errProbe
}

func init() {
	// Defined once for the package, the way the real modes are defined by their
	// own packages.
	Define(plugin.Mode{Name: "probe-standard", Node: "probe-standard-node", Summary: "the room as it comes"})
	Define(plugin.Mode{Name: "probe-creator", Node: "probe-creator-node", Summary: "the room with tools to grow"})
}

func TestSetSwitchesByMountingOneAndUnmountingTheOther(t *testing.T) {
	tree := &probeTree{mounted: []string{"probe-standard-node"}}
	r := New(tree)

	if got := r.Current().Name; got != "probe-standard" {
		t.Fatalf("current = %q, want probe-standard", got)
	}

	if err := r.Set("probe-creator"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := r.Current().Name; got != "probe-creator" {
		t.Fatalf("current = %q, want probe-creator", got)
	}
	for _, m := range tree.Mounted() {
		if m == "probe-standard-node" {
			t.Fatal("the mode we left is still mounted: one mode at a time")
		}
	}

	if err := r.Set("probe-standard"); err != nil {
		t.Fatalf("Set back: %v", err)
	}
	if got := r.Current().Name; got != "probe-standard" {
		t.Fatalf("current = %q, want probe-standard again", got)
	}
	for _, m := range tree.Mounted() {
		if m == "probe-creator-node" {
			t.Fatal("going back left the other mode mounted")
		}
	}
}

func TestAnUnknownModeIsRefusedAndChangesNothing(t *testing.T) {
	tree := &probeTree{mounted: []string{"probe-standard-node"}}
	r := New(tree)

	err := r.Set("nonsense")
	if err == nil {
		t.Fatal("switching to an unknown mode was allowed")
	}
	if !strings.Contains(err.Error(), "unknown mode") {
		t.Fatalf("error = %v, want it to say the mode is unknown", err)
	}
	if got := tree.Mounted(); len(got) != 1 || got[0] != "probe-standard-node" {
		t.Fatalf("the tree changed: %v", got)
	}
}

func TestAFailedSwitchLeavesTheModeInForce(t *testing.T) {
	tree := &probeTree{mounted: []string{"probe-standard-node"}, failMount: "probe-creator-node"}
	r := New(tree)

	if err := r.Set("probe-creator"); err == nil {
		t.Fatal("a failing mount was reported as a successful switch")
	}
	if got := r.Current().Name; got != "probe-standard" {
		t.Fatalf("current = %q, want the mode in force left alone", got)
	}
	if got := tree.Mounted(); len(got) != 1 || got[0] != "probe-standard-node" {
		t.Fatalf("a failed switch changed the tree: %v", got)
	}
}

func TestAllListsEveryModeDefined(t *testing.T) {
	got := New(nil).All()
	if len(got) < 2 {
		t.Fatalf("All returned %d modes, want at least the two probes", len(got))
	}
	if got[0].Name > got[1].Name {
		t.Fatalf("modes are not in name order: %v", got)
	}
}
