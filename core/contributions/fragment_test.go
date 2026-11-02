package contributions

import (
	"strings"
	"testing"

	"github.com/usually-broken/lola/core/plugin"
)

// A fragment says what it says now, not what it said when it was offered: the
// page reads the list on every poll, and a fragment showing a stale count is a
// fragment lying quietly.
func TestAFragmentIsDrawnFreshEachTime(t *testing.T) {
	r := New()
	shown := "first"
	r.Add(plugin.Contribution{
		Place: "above-composer",
		Kind:  plugin.KindFragment,
		Name:  "count",
		Draw:  func() string { return `<span>` + shown + `</span>` },
	})

	got := r.All()
	if len(got) != 1 {
		t.Fatalf("offered %d contributions, want 1", len(got))
	}
	if !strings.Contains(got[0].Value, "first") {
		t.Errorf("fragment drawn as %q, want first", got[0].Value)
	}

	shown = "second"
	again := r.All()
	if !strings.Contains(again[0].Value, "second") {
		t.Errorf("fragment drawn as %q on the second read, want second", again[0].Value)
	}
}

// A fragment is not changeable: there is nothing to press. If a plugin gives it
// nothing to change with, the frontend is told so rather than left to find out.
func TestAFragmentIsNotChangeable(t *testing.T) {
	r := New()
	r.Add(plugin.Contribution{
		Place: "above-composer",
		Kind:  plugin.KindFragment,
		Name:  "count",
		Draw:  func() string { return "x" },
	})

	if got := r.All(); got[0].Changeable {
		t.Error("a fragment with nothing to change it with was reported as changeable")
	}
}

// The kind is spelled once, in the package that defines the kinds, so a plugin
// offering a fragment cannot get the word wrong.
func TestTheFragmentKindIsNamedHere(t *testing.T) {
	if plugin.KindFragment != "fragment" {
		t.Errorf("KindFragment = %q, want fragment", plugin.KindFragment)
	}
}
