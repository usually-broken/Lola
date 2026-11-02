package websearch

import (
	"os"
	"strings"
	"testing"
)

// The parse is the part that can be wrong without the network, so it is tested
// against a real page saved to disk. The live call is in live_test.go.
func TestParseFindsTheAnswer(t *testing.T) {
	page, err := os.ReadFile("testdata/capital.html")
	if err != nil {
		t.Fatal(err)
	}

	results := Parse(string(page), 5)
	if len(results) == 0 {
		t.Fatal("no results parsed out of a real results page")
	}

	// A results page for "capital of ethiopia" leads with Addis Ababa. If the
	// parse is right, the word is in the top result.
	top := results[0].Title + " " + results[0].Snippet
	if !strings.Contains(top, "Addis Ababa") {
		t.Errorf("top result does not mention Addis Ababa: %q", top)
	}
}

func TestParseDropsSponsored(t *testing.T) {
	page, err := os.ReadFile("testdata/capital.html")
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range Parse(string(page), 10) {
		if strings.Contains(r.URL, "y.js") {
			t.Errorf("a sponsored result got through: %s", r.URL)
		}
	}
}

func TestParseIsLimited(t *testing.T) {
	page, err := os.ReadFile("testdata/capital.html")
	if err != nil {
		t.Fatal(err)
	}

	if got := len(Parse(string(page), 2)); got != 2 {
		t.Errorf("limit 2 gave %d results", got)
	}
}

func TestParseOnNothing(t *testing.T) {
	if got := Parse("<html><body>nothing here</body></html>", 5); len(got) != 0 {
		t.Errorf("expected no results, got %d", len(got))
	}
}
