package routes_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usually-broken/lola/core/routes"
)

// A path offered and then taken back must stop answering. A registry that only
// grows is how a plugin switched off keeps serving — which reads, from the
// other side of the screen, as a switch that does nothing.
func TestRemoveStopsAPathAnswering(t *testing.T) {
	set := routes.New()
	answering := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

	if err := set.Handle("/api/thing", answering); err != nil {
		t.Fatal(err)
	}
	if _, ok := set.Match("/api/thing"); !ok {
		t.Fatal("a offered path should answer")
	}

	set.Remove("/api/thing")

	if _, ok := set.Match("/api/thing"); ok {
		t.Error("a taken-back path should not answer")
	}
	if got := len(set.All()); got != 0 {
		t.Errorf("All() = %d routes, want none", got)
	}
}

// Removing one path leaves the others alone.
func TestRemoveLeavesOtherPaths(t *testing.T) {
	set := routes.New()
	answering := func(w http.ResponseWriter, _ *http.Request) {}

	for _, p := range []string{"/api/a", "/api/b"} {
		if err := set.Handle(p, answering); err != nil {
			t.Fatal(err)
		}
	}

	set.Remove("/api/a")

	if _, ok := set.Match("/api/b"); !ok {
		t.Error("/api/b should still answer")
	}
	if got := len(set.All()); got != 1 {
		t.Errorf("All() = %d routes, want 1", got)
	}
}

// And what it serves is still the plugin's own handler, not a stub.
func TestWhatIsServedIsWhatWasOffered(t *testing.T) {
	set := routes.New()
	if err := set.Handle("/api/thing", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("from the plugin"))
	}); err != nil {
		t.Fatal(err)
	}

	h, _ := set.Match("/api/thing")
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/thing", nil))

	if rec.Body.String() != "from the plugin" {
		t.Errorf("body = %q", rec.Body.String())
	}
}
