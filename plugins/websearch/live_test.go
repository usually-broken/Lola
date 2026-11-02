package websearch

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The real call, which needs the internet. It only runs when asked for, so a test
// suite on a train does not fail:
//
//	WEBSEARCH_LIVE=1 go test ./plugins/websearch/ -run Live -v
func TestLiveSearch(t *testing.T) {
	if os.Getenv("WEBSEARCH_LIVE") == "" {
		t.Skip("set WEBSEARCH_LIVE=1 to call the endpoint for real")
	}

	p := New(nil)
	out, err := p.Run(context.Background(), json.RawMessage(`{"query":"capital of ethiopia","limit":3}`))
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	t.Logf("tool returned:\n%s", out)

	if !strings.Contains(out, "Addis Ababa") {
		t.Errorf("the answer was not in the results")
	}
	if strings.Contains(out, "y.js") {
		t.Errorf("a sponsored result was reported")
	}
}
