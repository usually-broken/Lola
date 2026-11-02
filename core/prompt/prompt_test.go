package prompt

import (
	"strings"
	"testing"
)

func TestPartsAreReadInOrderNotInTheOrderTheyArrived(t *testing.T) {
	r := New()
	mustAdd(t, r, Part{Name: "late", Order: 20, Text: "second"})
	mustAdd(t, r, Part{Name: "early", Order: 10, Text: "first"})

	if got := r.Text(); got != "first\n\nsecond" {
		t.Errorf("assembled %q", got)
	}
}

// Two parts with the same name is a mistake: one would overwrite the other and the
// plugin that lost would never know.
func TestTwoPartsCannotClaimTheSameName(t *testing.T) {
	r := New()
	mustAdd(t, r, Part{Name: "persona", Text: "one"})

	if err := r.Add(Part{Name: "persona", Text: "two"}); err == nil {
		t.Fatal("a duplicate name was accepted")
	}
	if got := r.Text(); got != "one" {
		t.Errorf("the first part was disturbed: %q", got)
	}
}

func TestAPartNeedsANameAndSomeText(t *testing.T) {
	r := New()
	if err := r.Add(Part{Text: "no name"}); err == nil {
		t.Error("a nameless part was accepted")
	}
	if err := r.Add(Part{Name: "empty", Text: "   "}); err == nil {
		t.Error("an empty part was accepted")
	}
}

func TestRemovingAPartTakesItOutOfThePrompt(t *testing.T) {
	r := New()
	mustAdd(t, r, Part{Name: "stay", Order: 1, Text: "kept"})
	mustAdd(t, r, Part{Name: "go", Order: 2, Text: "gone"})

	if !r.Remove("go") {
		t.Fatal("Remove said the part was not there")
	}
	if r.Remove("go") {
		t.Error("Remove succeeded twice for one part")
	}
	if got := r.Text(); got != "kept" {
		t.Errorf("assembled %q after removal", got)
	}
}

// Nothing contributing means no system prompt at all, which is different from an
// empty one: the caller has to be able to tell.
func TestNoPartsMeansNoText(t *testing.T) {
	if got := New().Text(); got != "" {
		t.Errorf("empty registry assembled %q", got)
	}
}

func TestEqualOrdersDoNotDependOnMountOrder(t *testing.T) {
	first := New()
	mustAdd(t, first, Part{Name: "b", Order: 1, Text: "bee"})
	mustAdd(t, first, Part{Name: "a", Order: 1, Text: "ay"})

	second := New()
	mustAdd(t, second, Part{Name: "a", Order: 1, Text: "ay"})
	mustAdd(t, second, Part{Name: "b", Order: 1, Text: "bee"})

	if first.Text() != second.Text() {
		t.Errorf("mount order changed the prompt: %q vs %q", first.Text(), second.Text())
	}
	if !strings.HasPrefix(first.Text(), "ay") {
		t.Errorf("equal orders should fall back to name, got %q", first.Text())
	}
}

func mustAdd(t *testing.T, r *Registry, part Part) {
	t.Helper()
	if err := r.Add(part); err != nil {
		t.Fatalf("Add(%q): %v", part.Name, err)
	}
}

// A part that describes something which does not exist at mount time has to answer
// when asked. The mode in force is the case that found this: asking during the build
// looked up a tree that was still being assembled.
func TestALivePartAnswersWhenRead(t *testing.T) {
	r := New()
	state := "first"

	if err := r.Add(Part{Name: "live", Live: func() string { return state }}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got := r.Text(); got != "first" {
		t.Errorf("assembled %q", got)
	}

	state = "second"
	if got := r.Text(); got != "second" {
		t.Errorf("a live part did not follow its source: %q", got)
	}
	if parts := r.Parts(); parts[0].Text != "second" {
		t.Errorf("Parts did not fill in live text: %q", parts[0].Text)
	}
}

func TestALivePartThatSaysNothingIsDropped(t *testing.T) {
	r := New()
	mustAdd(t, r, Part{Name: "always", Order: 1, Text: "kept"})
	if err := r.Add(Part{Name: "quiet", Order: 2, Live: func() string { return "  " }}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if got := r.Text(); got != "kept" {
		t.Errorf("assembled %q", got)
	}
}
