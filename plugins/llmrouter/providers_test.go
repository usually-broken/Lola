package llmrouter

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/usually-broken/lola/core/llm"
)

// recorder is a provider that remembers which entry it was built for, so a
// switch can be shown to reach the next call rather than the next restart.
type recorder struct {
	entry Entry
	key   string
}

func (r *recorder) Name() string { return "recorder" }

func (r *recorder) Complete(context.Context, llm.Request) (llm.Response, error) {
	return llm.Response{Content: r.entry.Name}, nil
}

// builder builds recorders and remembers the keys they were handed.
func builder(keys map[string]string) Builder {
	return func(e Entry, key string) llm.Provider {
		keys[e.Name] = key
		return &recorder{entry: e, key: key}
	}
}

func twoProviders(t *testing.T) (*Registry, map[string]string) {
	t.Helper()

	keys := map[string]string{}
	r, err := New(builder(keys),
		Entry{Name: "local", URL: "http://127.0.0.1:11434/v1"},
		Entry{Name: "cloud", URL: "https://example.invalid/v1"},
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r, keys
}

func TestTheFirstProviderIsTheOneInUse(t *testing.T) {
	r, _ := twoProviders(t)

	if got := r.Current(); got != "local" {
		t.Fatalf("current = %q, want local", got)
	}
}

// The point of the seam: a switch has to reach the next call, or the endpoint is
// still fixed in practice.
func TestSwitchingReachesTheNextCall(t *testing.T) {
	r, _ := twoProviders(t)

	res, err := r.Complete(context.Background(), llm.Request{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.Content != "local" {
		t.Fatalf("the first call went to %q, want local", res.Content)
	}

	if err := r.Set("cloud"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	res, err = r.Complete(context.Background(), llm.Request{})
	if err != nil {
		t.Fatalf("Complete after the switch: %v", err)
	}
	if res.Content != "cloud" {
		t.Fatalf("the call after the switch went to %q, want cloud", res.Content)
	}
}

func TestAnUnknownProviderIsRefusedAndChangesNothing(t *testing.T) {
	r, _ := twoProviders(t)

	if err := r.Set("nonsense"); err == nil {
		t.Fatal("switching to an unknown provider was allowed")
	}
	if got := r.Current(); got != "local" {
		t.Fatalf("current = %q, want it left alone", got)
	}
}

func TestAKeyIsReadFromTheEnvironmentAtCallTime(t *testing.T) {
	t.Setenv("PROVTEST_API_KEY", "from-the-environment")

	entries, err := Entries([]Spec{{Name: "provtest", URL: "https://example.invalid/v1", Key: "PROVTEST_API_KEY"}})
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}

	keys := map[string]string{}
	r, err := New(builder(keys), entries...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := r.Complete(context.Background(), llm.Request{}); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if keys["provtest"] != "from-the-environment" {
		t.Fatalf("the key handed to the provider was %q", keys["provtest"])
	}
}

// A key written into the config is used as it is, and it never goes near a page.
func TestALiteralKeyIsUsedAsIsAndNeverSerialised(t *testing.T) {
	const secret = "sk-live-do-not-leak-me"

	entries, err := Entries([]Spec{{Name: "cloud", URL: "https://example.invalid/v1", Key: secret}})
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if entries[0].KeyFrom != "config" {
		t.Fatalf("keyFrom = %q, want config", entries[0].KeyFrom)
	}

	keys := map[string]string{}
	r, err := New(builder(keys), entries...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := r.Complete(context.Background(), llm.Request{}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if keys["cloud"] != secret {
		t.Fatal("a literal key was not handed to the provider")
	}

	wire, err := json.Marshal(r.All())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(wire), secret) {
		t.Fatalf("the key was serialised: %s", wire)
	}
	if !strings.Contains(string(wire), "config") {
		t.Fatalf("a page cannot tell where the key came from: %s", wire)
	}
}

func TestMissingKeyIsNotAnErrorSoLocalEndpointsStillWork(t *testing.T) {
	os.Unsetenv("NOKEY_API_KEY")

	entries, err := Entries([]Spec{{Name: "nokey", URL: "http://127.0.0.1:11434/v1", Key: "NOKEY_API_KEY"}})
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}

	keys := map[string]string{}
	r, err := New(builder(keys), entries...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := r.Complete(context.Background(), llm.Request{}); err != nil {
		t.Fatalf("a local endpoint with no key should still work: %v", err)
	}
	if keys["nokey"] != "" {
		t.Fatalf("a key was invented: %q", keys["nokey"])
	}
}

func TestNewRefusesWhatItCannotUse(t *testing.T) {
	cases := []struct {
		what    string
		entries []Entry
	}{
		{"nothing at all", nil},
		{"an entry with no name", []Entry{{URL: "http://127.0.0.1:11434/v1"}}},
		{"an entry with no url", []Entry{{Name: "local"}}},
		{"the same name twice", []Entry{{Name: "local", URL: "a"}, {Name: "local", URL: "b"}}},
	}

	for _, c := range cases {
		if _, err := New(nil, c.entries...); err == nil {
			t.Errorf("New accepted %s", c.what)
		}
	}
}

// The rule for keys: a variable's name is read from the environment, and
// anything else is the key itself. The safe reading is also the natural one.
func TestEntriesWorksOutWhereAKeyComesFrom(t *testing.T) {
	entries, err := Entries([]Spec{
		{Name: "local", URL: "http://127.0.0.1:11434/v1"},
		{Name: "openrouter", URL: "https://openrouter.ai/api/v1", Key: "OPENROUTER_API_KEY"},
		{Name: "pasted", URL: "https://example.invalid/v1", Key: "sk-abc-123"},
	})
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}

	if entries[0].KeyFrom != "" {
		t.Errorf("a provider with no key says its key comes from %q", entries[0].KeyFrom)
	}
	if entries[1].KeyFrom != "OPENROUTER_API_KEY" {
		t.Errorf("a variable name was not recognised: %q", entries[1].KeyFrom)
	}
	if entries[2].KeyFrom != "config" {
		t.Errorf("a pasted key was not recognised: %q", entries[2].KeyFrom)
	}

	if _, err := Entries([]Spec{{Name: "", URL: "x"}}); err == nil {
		t.Error("Entries accepted an entry with no name")
	}
	if _, err := Entries([]Spec{{Name: "x"}}); err == nil {
		t.Error("Entries accepted an entry with no url")
	}
}
