package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// A tool that records what it was given, so the test can see what reached it.
type recorder struct {
	called bool
	args   map[string]any
}

func (r *recorder) Name() string        { return "recorder" }
func (r *recorder) Description() string { return "records" }
func (r *recorder) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}
func (r *recorder) Run(_ context.Context, args json.RawMessage) (string, error) {
	r.called = true
	_ = json.Unmarshal(args, &r.args)
	return "ok", nil
}

func registryWith(t *testing.T, tool Tool) *Registry {
	t.Helper()
	r := New()
	if _, err := r.Register(tool); err != nil {
		t.Fatal(err)
	}
	return r
}

// The rule: a call with no reason does not run.
func TestACallWithoutAReasonIsRefused(t *testing.T) {
	rec := &recorder{}
	r := registryWith(t, rec)

	_, err := r.Run(context.Background(), "recorder", json.RawMessage(`{}`))

	if !errors.Is(err, ErrNoReason) {
		t.Fatalf("expected ErrNoReason, got %v", err)
	}
	if rec.called {
		t.Error("the tool ran anyway")
	}
}

func TestAnEmptyReasonIsRefused(t *testing.T) {
	rec := &recorder{}
	r := registryWith(t, rec)

	_, err := r.Run(context.Background(), "recorder", json.RawMessage(`{"why":"   "}`))

	if !errors.Is(err, ErrNoReason) {
		t.Fatalf("expected ErrNoReason, got %v", err)
	}
	if rec.called {
		t.Error("the tool ran anyway")
	}
}

func TestAReasonLetsItRun(t *testing.T) {
	rec := &recorder{}
	r := registryWith(t, rec)

	out, err := r.Run(context.Background(), "recorder",
		json.RawMessage(`{"why":"checking the date, because I don't know it","path":"x"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ok" || !rec.called {
		t.Fatalf("tool did not run: out=%q called=%v", out, rec.called)
	}
	if rec.args["why"] == nil {
		t.Error("the reason did not reach the tool, so it cannot be shown when approving")
	}
}

// The schema the model sees asks for the reason, on every tool, without the
// plugin doing anything.
func TestWithWhyAddsARequiredReason(t *testing.T) {
	got := WithWhy(json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`))

	var doc struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(got, &doc); err != nil {
		t.Fatalf("generated an unreadable schema: %v", err)
	}

	if _, ok := doc.Properties["why"]; !ok {
		t.Error("no why property was added")
	}
	required := strings.Join(doc.Required, ",")
	if !strings.Contains(required, "why") || !strings.Contains(required, "path") {
		t.Errorf("required = %q, want both why and path", required)
	}
}

func TestWithWhyIsIdempotent(t *testing.T) {
	once := WithWhy(json.RawMessage(`{"type":"object","properties":{}}`))
	twice := WithWhy(once)

	if strings.Count(string(twice), `"why"`) != strings.Count(string(once), `"why"`) {
		t.Error("asking twice added the reason twice")
	}
}

func TestWithWhyLeavesAnUnreadableSchemaAlone(t *testing.T) {
	bad := json.RawMessage(`not json`)
	if got := WithWhy(bad); string(got) != string(bad) {
		t.Errorf("an unreadable schema was rewritten: %s", got)
	}
}
