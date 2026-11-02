package contributions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/usually-broken/lola/core/plugin"
)

// What a plugin offers comes back, in the order it was offered, because that is
// the order an interface draws them in.
func TestWhatIsOfferedComesBackInOrder(t *testing.T) {
	r := New()
	r.Add(plugin.Contribution{Place: "above-composer", Kind: plugin.KindChip, Name: "one", Label: "One"})
	r.Add(plugin.Contribution{Place: "above-composer", Kind: plugin.KindToggle, Name: "two", Label: "Two"})

	got := r.All()
	if len(got) != 2 {
		t.Fatalf("got %d contributions, want 2", len(got))
	}
	if got[0].Name != "one" || got[1].Name != "two" {
		t.Fatalf("got %q then %q, want one then two", got[0].Name, got[1].Name)
	}
}

// A contribution with no place, no kind or no name cannot be drawn, so it is
// refused rather than stored. A silent no-show in an interface is a miserable
// thing to track down.
func TestWhatCannotBeDrawnIsRefused(t *testing.T) {
	cases := map[string]plugin.Contribution{
		"no place": {Kind: plugin.KindChip, Name: "a", Label: "A"},
		"no kind":  {Place: "above-composer", Name: "b", Label: "B"},
		"no name":  {Place: "above-composer", Kind: plugin.KindChip, Label: "C"},
	}

	for name, c := range cases {
		r := New()
		r.Add(c)
		if len(r.All()) != 0 {
			t.Errorf("%s: stored a contribution that cannot be drawn", name)
		}
	}
}

// A value that can change is read at the moment it is asked for, so an
// interface never shows a mode that has already been switched away from.
func TestAChangeableValueIsReadFresh(t *testing.T) {
	current := "standard"
	r := New()
	r.Add(plugin.Contribution{
		Place: "above-composer",
		Kind:  plugin.KindSelect,
		Name:  "mode",
		Label: "Mode",
		Current: func() (string, []string) {
			return current, []string{"standard", "creator"}
		},
	})

	if got := r.All()[0].Value; got != "standard" {
		t.Fatalf("got %q, want standard", got)
	}

	current = "creator"

	got := r.All()[0]
	if got.Value != "creator" {
		t.Fatalf("got %q after the change, want creator", got.Value)
	}
	if len(got.Options) != 2 {
		t.Fatalf("got %d options, want 2", len(got.Options))
	}
}

// The reader is Go-side only. If it ever travelled, the page would be sent a
// function and the whole response would fail to encode.
func TestTheReaderNeverTravels(t *testing.T) {
	r := New()
	r.Add(plugin.Contribution{
		Place:   "above-composer",
		Kind:    plugin.KindSelect,
		Name:    "mode",
		Label:   "Mode",
		Current: func() (string, []string) { return "standard", []string{"standard"} },
	})

	out, err := json.Marshal(r.All())
	if err != nil {
		t.Fatalf("encoding what a plugin offered failed: %v", err)
	}
	if strings.Contains(string(out), "Current") || strings.Contains(string(out), "func") {
		t.Fatalf("the reader travelled to the page: %s", out)
	}
	if !strings.Contains(string(out), `"value":"standard"`) {
		t.Fatalf("the value did not travel: %s", out)
	}
}

// Whether a thing can be changed is said, not guessed at: a frontend is told,
// so it can draw a control that can be pressed or a label that cannot.
func TestBeingChangeableIsSaidOutLoud(t *testing.T) {
	r := New()
	r.Add(plugin.Contribution{Place: "above-composer", Kind: plugin.KindText, Name: "readonly", Label: "Read only"})
	r.Add(plugin.Contribution{
		Place: "above-composer",
		Kind:  plugin.KindToggle,
		Name:  "switch",
		Label: "Switch",
		Set:   func(value string) error { return nil },
	})

	got := r.All()
	if got[0].Changeable {
		t.Errorf("%q is offered to be read, but is reported as changeable", got[0].Name)
	}
	if !got[1].Changeable {
		t.Errorf("%q has something to change with, but is not reported as changeable", got[1].Name)
	}
}

// A kind the page does not know still travels. Leaving it out here would hide
// a plugin's work from every frontend at once, which is worse than a frontend
// deciding for itself that it cannot draw it.
func TestAnUnknownKindStillTravels(t *testing.T) {
	r := New()
	r.Add(plugin.Contribution{Place: "above-composer", Kind: "graph", Name: "ttft", Label: "TTFT"})

	out, err := json.Marshal(r.All())
	if err != nil {
		t.Fatalf("encoding failed: %v", err)
	}
	if !strings.Contains(string(out), `"kind":"graph"`) {
		t.Fatalf("an unknown kind was dropped: %s", out)
	}
}
