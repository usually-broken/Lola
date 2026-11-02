package webfetch

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Stripping is the part that can be wrong without a network, so it runs against
// saved markup.
func TestToTextDropsMarkupAndScripts(t *testing.T) {
	page := `<html><head><style>.x{color:red}</style>
	<script>var noise = "should not appear";</script></head>
	<body><h1>A heading</h1><p>First para with <b>bold</b>.</p>
	<!-- a comment -->
	<p>Second&nbsp;para &amp; an entity.</p><br><p>Third.</p></body></html>`

	got := ToText(page, "text/html; charset=utf-8")

	for _, want := range []string{"A heading", "First para with bold.", "Second para & an entity.", "Third."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"should not appear", "color:red", "a comment", "<p>"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("kept %q in output:\n%s", unwanted, got)
		}
	}
}

func TestToTextPlainIsUntouched(t *testing.T) {
	got := ToText("just text\nsecond line", "text/plain")
	if got != "just text\nsecond line" {
		t.Errorf("plain text was rewritten: %q", got)
	}
}

func TestToTextOnNothingReadable(t *testing.T) {
	if got := ToText(`<html><script>only code</script></html>`, "text/html"); strings.TrimSpace(got) != "" {
		t.Errorf("expected nothing readable, got %q", got)
	}
}

// The real call, only when asked for:
//
//	WEBFETCH_LIVE=1 go test ./plugins/webfetch/ -run Live -v
func TestLiveFetch(t *testing.T) {
	if os.Getenv("WEBFETCH_LIVE") == "" {
		t.Skip("set WEBFETCH_LIVE=1 to fetch a real page")
	}

	p := New(nil)
	out, err := p.Run(context.Background(), json.RawMessage(`{"url":"https://go.dev/blog/","chars":2000}`))
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	t.Logf("first lines:\n%s", head(out, 400))
	if strings.Contains(out, "<div") || strings.Contains(out, "<script") {
		t.Errorf("markup survived into the text")
	}
}

func head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
