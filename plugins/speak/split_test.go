package speak

import (
	"strings"
	"testing"
)

func TestSentencesEndAtFullStops(t *testing.T) {
	got := Split("One thing. Then another. And a third!")
	want := []string{"One thing.", "Then another.", "And a third!"}

	if len(got) != len(want) {
		t.Fatalf("got %d sentences: %q", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sentence %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// A decimal point is not the end of a sentence, or "three point five" becomes two
// sentences and gets spoken with a silly pause in the middle.
func TestADecimalIsNotASentenceEnd(t *testing.T) {
	got := Split("It costs 3.5 pounds, which is fine.")
	if len(got) != 1 {
		t.Errorf("a decimal split the sentence: %q", got)
	}
}

func TestAnAbbreviationIsNotASentenceEnd(t *testing.T) {
	got := Split("Mr. Smith said e.g. this and that.")
	if len(got) != 1 {
		t.Errorf("an abbreviation split the sentence: %q", got)
	}
}

// A model that forgets punctuation still has to be spoken in pieces, cut between
// words rather than through them.
func TestARunOnIsCutAtAWord(t *testing.T) {
	long := strings.Repeat("word ", 90) // 450 characters, no punctuation
	got := Split(strings.TrimSpace(long))

	if len(got) < 2 {
		t.Fatalf("a run-on was not cut: %d piece(s)", len(got))
	}
	for i, piece := range got {
		if strings.HasSuffix(piece, "wor") || strings.HasPrefix(piece, "d ") {
			t.Errorf("piece %d was cut through a word: %q", i, piece)
		}
	}
	if rejoined := strings.Join(got, " "); rejoined != strings.TrimSpace(long) {
		t.Errorf("the text changed when it was cut:\n%q\n%q", rejoined, strings.TrimSpace(long))
	}
}

func TestEmptyTextSaysNothing(t *testing.T) {
	if got := Split("   "); len(got) != 0 {
		t.Errorf("blank text produced %q", got)
	}
}

func TestATrailingFragmentIsKept(t *testing.T) {
	got := Split("One. Two")
	if len(got) != 2 || got[1] != "Two" {
		t.Errorf("got %q", got)
	}
}
