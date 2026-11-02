package tools

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

type fake struct {
	name string
	out  string
	err  error
}

func (f *fake) Name() string            { return f.name }
func (f *fake) Description() string     { return "fake tool" }
func (f *fake) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f *fake) Run(context.Context, json.RawMessage) (string, error) {
	return f.out, f.err
}

func TestRegisterAndRun(t *testing.T) {
	r := New()
	if _, err := r.Register(&fake{name: "echo", out: "hi"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := r.Run(context.Background(), "echo", json.RawMessage(`{"why":"testing the registry"}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "hi" {
		t.Fatalf("out = %q, want %q", got, "hi")
	}
}

func TestRegisterReturnsWorkingTeardown(t *testing.T) {
	r := New()
	teardown, err := r.Register(&fake{name: "echo"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if r.Len() != 1 {
		t.Fatalf("Len = %d, want 1", r.Len())
	}

	if err := teardown(); err != nil {
		t.Fatalf("teardown: %v", err)
	}
	if r.Len() != 0 {
		t.Fatalf("Len = %d after teardown, want 0", r.Len())
	}
	if _, ok := r.Get("echo"); ok {
		t.Fatal("tool still registered after teardown")
	}
}

func TestDuplicateNameRejected(t *testing.T) {
	r := New()
	if _, err := r.Register(&fake{name: "echo"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, err := r.Register(&fake{name: "echo"})
	if !errors.Is(err, ErrDuplicateTool) {
		t.Fatalf("err = %v, want ErrDuplicateTool", err)
	}
}

func TestRunUnknownTool(t *testing.T) {
	r := New()
	_, err := r.Run(context.Background(), "nope", nil)
	if !errors.Is(err, ErrUnknownTool) {
		t.Fatalf("err = %v, want ErrUnknownTool", err)
	}
}

func TestNamesAreSorted(t *testing.T) {
	r := New()
	for _, n := range []string{"c", "a", "b"} {
		if _, err := r.Register(&fake{name: n}); err != nil {
			t.Fatalf("Register %s: %v", n, err)
		}
	}
	if got, want := r.Names(), []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
}

func TestRunUnwrapsStringEncodedArguments(t *testing.T) {
	// Some providers return tool arguments as a JSON string containing JSON
	// rather than as an object. The registry must hand the tool the object.
	r := New()
	var seen string

	tool := &captureTool{seen: &seen}
	if _, err := r.Register(tool); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if _, err := r.Run(context.Background(), "capture", json.RawMessage(`"{\"path\":\"x\",\"why\":\"testing unwrapping\"}"`)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seen != `{"path":"x","why":"testing unwrapping"}` {
		t.Fatalf("tool saw %q, want the unwrapped object", seen)
	}
}

// captureTool records the raw arguments it was handed.
type captureTool struct{ seen *string }

func (c *captureTool) Name() string            { return "capture" }
func (c *captureTool) Description() string     { return "captures its arguments" }
func (c *captureTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (c *captureTool) Run(_ context.Context, args json.RawMessage) (string, error) {
	*c.seen = string(args)
	return "ok", nil
}

func TestRegistryIsSafeForConcurrentUse(t *testing.T) {
	r := New()
	done := make(chan struct{})

	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			teardown, err := r.Register(&fake{name: "echo"})
			if err != nil {
				continue
			}
			_ = teardown()
		}
	}()

	for i := 0; i < 100; i++ {
		_ = r.Names()
		_, _ = r.Get("echo")
	}
	<-done
}
