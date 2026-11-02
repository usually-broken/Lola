package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// answer writes its own name, so a test can tell which handler ran.
func answer(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(name))
	}
}

// ask sends one request through a handler and returns what came back.
func ask(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// A plugin offers a path and gets it back when a request arrives for it.
func TestAPluginGetsThePathItOffered(t *testing.T) {
	r := New()
	if err := r.Handle("/api/health", answer("health")); err != nil {
		t.Fatalf("offering a path failed: %v", err)
	}

	if got, ok := r.Match("/api/health"); !ok {
		t.Fatal("the offered path does not match")
	} else if rec := ask(t, got, "/api/health"); rec.Body.String() != "health" {
		t.Errorf("path answered with %q, want health", rec.Body.String())
	}
}

// An exact pattern is exact: it does not quietly swallow everything below it.
func TestAnExactPatternIsExact(t *testing.T) {
	r := New()
	if err := r.Handle("/api/health", answer("health")); err != nil {
		t.Fatal(err)
	}

	if _, ok := r.Match("/api/health/deeper"); ok {
		t.Error("an exact pattern claimed a path below it")
	}
}

// A pattern ending in a slash covers what is under it, which is how a plugin
// that owns a whole area — a folder of its own assets, say — offers it once.
func TestAPatternEndingInASlashCoversWhatIsUnderIt(t *testing.T) {
	r := New()
	if err := r.Handle("/panel/composer/", answer("composer")); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/panel/composer/", "/panel/composer/thing.js"} {
		if _, ok := r.Match(path); !ok {
			t.Errorf("%s was not claimed by the pattern that covers it", path)
		}
	}
	if _, ok := r.Match("/panel/other"); ok {
		t.Error("the pattern claimed something outside it")
	}
}

// The longest pattern wins, so a plugin can carve one path out of a wider area
// another plugin owns.
func TestTheLongestPatternWins(t *testing.T) {
	r := New()
	if err := r.Handle("/panel/", answer("wide")); err != nil {
		t.Fatal(err)
	}
	if err := r.Handle("/panel/composer/", answer("narrow")); err != nil {
		t.Fatal(err)
	}

	got, ok := r.Match("/panel/composer/edit")
	if !ok {
		t.Fatal("nothing claimed the path")
	}
	if rec := ask(t, got, "/panel/composer/edit"); rec.Body.String() != "narrow" {
		t.Errorf("answered with %q, want narrow", rec.Body.String())
	}
}

// A path that could never work is refused when it is offered, not when it is
// asked for.
func TestAPathThatCouldNeverWorkIsRefused(t *testing.T) {
	r := New()

	if err := r.Handle("", answer("x")); err == nil {
		t.Error("an empty path was accepted")
	}
	if err := r.Handle("api/health", answer("x")); err == nil {
		t.Error("a path with no leading slash was accepted")
	}
	if err := r.Handle("/api/health", nil); err == nil {
		t.Error("a path with no handler was accepted")
	}
	if len(r.All()) != 0 {
		t.Errorf("a refused offer was kept anyway: %+v", r.All())
	}
}

// Everything the frontend already answers keeps working: a path no plugin
// claims falls through untouched.
func TestAnUnclaimedPathFallsThrough(t *testing.T) {
	r := New()
	if err := r.Handle("/api/health", answer("health")); err != nil {
		t.Fatal(err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("the frontend"))
	})
	served := r.Serve(next)

	if rec := ask(t, served, "/api/health"); rec.Body.String() != "health" {
		t.Errorf("a plugin's path was not handed over: %q", rec.Body.String())
	}
	if rec := ask(t, served, "/api/state"); rec.Body.String() != "the frontend" {
		t.Errorf("an unclaimed path did not fall through: %q", rec.Body.String())
	}
}

// A path offered after the frontend started is still answered, because the
// lookup happens per request. This is what lets a plugin be added to a running
// harness rather than only to a new one.
func TestAPathOfferedAfterTheFactIsAnswered(t *testing.T) {
	r := New()
	served := r.Serve(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("the frontend"))
	}))

	if rec := ask(t, served, "/api/health"); rec.Body.String() != "the frontend" {
		t.Fatalf("before anything was offered: %q", rec.Body.String())
	}

	if err := r.Handle("/api/health", answer("health")); err != nil {
		t.Fatal(err)
	}
	if rec := ask(t, served, "/api/health"); rec.Body.String() != "health" {
		t.Errorf("a path offered later was not answered: %q", rec.Body.String())
	}
}

// A plugin that offers a path and is offered it back by name: All is what a
// frontend lists, and it must not be a live view of the registry.
func TestAllIsACopy(t *testing.T) {
	r := New()
	if err := r.Handle("/a", answer("a")); err != nil {
		t.Fatal(err)
	}

	got := r.All()
	got[0].Pattern = "/tampered"
	if strings.Contains(r.All()[0].Pattern, "tampered") {
		t.Error("All handed out the registry's own records")
	}
}
