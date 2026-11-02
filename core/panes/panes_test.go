package panes

import (
	"testing"

	"github.com/usually-broken/lola/core/plugin"
)

func TestAddKeepsTheOrderTheyArrived(t *testing.T) {
	r := New()
	r.Add(plugin.Pane{Slot: "details", Title: "first"})
	r.Add(plugin.Pane{Slot: "details", Title: "second"})

	got := r.All()
	if len(got) != 2 {
		t.Fatalf("want 2 panes, got %d", len(got))
	}
	if got[0].Title != "first" || got[1].Title != "second" {
		t.Fatalf("order was not kept: %v", got)
	}
}

func TestAddIgnoresAPaneWithNowhereToGo(t *testing.T) {
	r := New()
	r.Add(plugin.Pane{Title: "no slot"})
	r.Add(plugin.Pane{Slot: "details"})

	// A pane the interface cannot place or label would render as a blank
	// panel, which is worse than no panel at all.
	if len(r.All()) != 0 {
		t.Fatalf("a pane with no slot or no title should be dropped, got %v", r.All())
	}
}

func TestAllGivesACopy(t *testing.T) {
	r := New()
	r.Add(plugin.Pane{Slot: "details", Title: "one"})

	got := r.All()
	got[0].Title = "meddled with"

	if r.All()[0].Title != "one" {
		t.Fatal("All handed out the registry's own slice")
	}
}
