package llmrouter

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func tempStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "providers.yaml")
	return NewStore(path, nil), path
}

func TestAddedProvidersAreSavedAndReadBack(t *testing.T) {
	store, path := tempStore(t)

	if err := store.Add(Spec{Name: "openrouter", URL: "https://openrouter.ai/api/v1", Key: "OPENROUTER_API_KEY"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// The file is written in the same shape the config uses, so the same parser
	// reads it back — one format, one thing to learn.
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading what was saved: %v", err)
	}
	for _, want := range []string{"providers:", "name: openrouter", "url: https://openrouter.ai/api/v1", "key: OPENROUTER_API_KEY"} {
		if !strings.Contains(string(written), want) {
			t.Fatalf("the saved file is missing %q:\n%s", want, written)
		}
	}

	if got := store.All(); len(got) != 1 || got[0].Name != "openrouter" {
		t.Fatalf("All returned %+v", got)
	}

	// A second store over the same file sees what the first wrote, which is what
	// makes it survive a restart.
	again := NewStore(path, store.All())
	if got := again.All(); len(got) != 1 || got[0].URL != "https://openrouter.ai/api/v1" {
		t.Fatalf("the saved provider did not come back: %+v", got)
	}
}

func TestAddingTheSameNameTwiceReplacesRatherThanDuplicates(t *testing.T) {
	store, _ := tempStore(t)

	if err := store.Add(Spec{Name: "cloud", URL: "https://wrong.invalid/v1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := store.Add(Spec{Name: "cloud", URL: "https://right.invalid/v1"}); err != nil {
		t.Fatalf("correcting it: %v", err)
	}

	got := store.All()
	if len(got) != 1 {
		t.Fatalf("the same name is in there twice: %+v", got)
	}
	if got[0].URL != "https://right.invalid/v1" {
		t.Fatalf("the correction did not land: %+v", got)
	}
}

func TestRemovingTakesItOutOfTheFileToo(t *testing.T) {
	store, path := tempStore(t)

	if err := store.Add(Spec{Name: "a", URL: "https://a.invalid/v1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := store.Add(Spec{Name: "b", URL: "https://b.invalid/v1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	was, err := store.Remove("a")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !was {
		t.Fatal("Remove said the provider was not there")
	}

	if got := store.All(); len(got) != 1 || got[0].Name != "b" {
		t.Fatalf("after removing a: %+v", got)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading what was saved: %v", err)
	}
	if strings.Contains(string(written), "https://a.invalid") {
		t.Fatalf("the removed provider is still in the file:\n%s", written)
	}

	// Removing something that isn't there says so rather than failing.
	was, err = store.Remove("nonsense")
	if err != nil || was {
		t.Fatalf("Remove of an unknown name returned (%v, %v)", was, err)
	}
}

func TestTheSavedFileIsNotWorldReadable(t *testing.T) {
	// Windows has no POSIX permission bits — everything reports 0666 whatever
	// was asked for — so this only means something where they exist.
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX permission bits on this platform")
	}

	store, path := tempStore(t)

	if err := store.Add(Spec{Name: "cloud", URL: "https://example.invalid/v1", Key: "SOMETHING"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("the saved file is %o, want 600", perm)
	}
}

// A provider added at runtime is saved, and takes effect straight away.
func TestAddMakesItAvailableAndRemembersIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.yaml")

	keys := map[string]string{}
	r, err := New(builder(keys), Entry{Name: "local", URL: "http://127.0.0.1:11434/v1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	r.Keep(NewStore(path, nil))

	if _, err := r.Add(Spec{Name: "cloud", URL: "https://example.invalid/v1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if got := len(r.All()); got != 2 {
		t.Fatalf("the registry has %d providers, want 2", got)
	}

	if err := r.Set("cloud"); err != nil {
		t.Fatalf("switching to the one just added: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("nothing was saved: %v", err)
	}
}

func TestRemovingTheOnlyProviderIsRefused(t *testing.T) {
	r, err := New(nil, Entry{Name: "local", URL: "http://127.0.0.1:11434/v1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := r.Remove("local"); err == nil {
		t.Fatal("removing the last provider was allowed, which leaves calls going nowhere")
	}
	if got := r.Current(); got != "local" {
		t.Fatalf("current = %q, want local", got)
	}
}

func TestRemovingTheOneInUseHandsOver(t *testing.T) {
	r, err := New(nil,
		Entry{Name: "local", URL: "http://127.0.0.1:11434/v1"},
		Entry{Name: "cloud", URL: "https://example.invalid/v1"},
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := r.Set("cloud"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := r.Remove("cloud"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if got := r.Current(); got != "local" {
		t.Fatalf("current = %q, want it to hand over to local", got)
	}
}
