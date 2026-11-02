package readfile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usually-broken/lola/core/tools"
	"github.com/usually-broken/lola/core/workspace"
)

// args builds tool arguments properly, so a Windows path's backslashes are
// escaped rather than glued into the JSON by hand.
func jsonArgs(t *testing.T, m map[string]string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRunReadsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("hello file"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := New(tools.New(), nil)
	got, err := p.Run(context.Background(), jsonArgs(t, map[string]string{"path": path}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "hello file" {
		t.Fatalf("out = %q, want %q", got, "hello file")
	}
}

func TestSetupRegistersAndTeardownRemoves(t *testing.T) {
	reg := tools.New()
	p := New(reg, nil)

	teardown, err := p.Setup()
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if _, ok := reg.Get("read_file"); !ok {
		t.Fatal("tool not registered after Setup")
	}
	if err := teardown(); err != nil {
		t.Fatalf("teardown: %v", err)
	}
	if _, ok := reg.Get("read_file"); ok {
		t.Fatal("tool still registered after teardown")
	}
}

func TestRunMissingPathIsAnError(t *testing.T) {
	p := New(tools.New(), nil)
	if _, err := p.Run(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected an error when path is missing")
	}
}

func TestRunTruncatesLargeFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", defaultMaxBytes+500)), 0o644); err != nil {
		t.Fatal(err)
	}

	p := New(tools.New(), nil)
	got, err := p.Run(context.Background(), jsonArgs(t, map[string]string{"path": path}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(got, "…(truncated)") {
		t.Fatal("expected large output to be truncated")
	}
}

func TestRunRefusesPathOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	ws := workspace.MustNew(root)

	// A file outside the workspace.
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := New(tools.New(), ws)
	_, err := p.Run(context.Background(), jsonArgs(t, map[string]string{"path": outside}))
	if err == nil {
		t.Fatal("reading outside the workspace should be refused")
	}
	if !strings.Contains(err.Error(), "outside the workspace") {
		t.Fatalf("error should explain the refusal, got: %v", err)
	}
}

func TestRunAllowsPathInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	ws := workspace.MustNew(root)

	inside := filepath.Join(root, "fine.txt")
	if err := os.WriteFile(inside, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := New(tools.New(), ws)
	got, err := p.Run(context.Background(), jsonArgs(t, map[string]string{"path": inside}))
	if err != nil {
		t.Fatalf("reading inside the workspace should work: %v", err)
	}
	if got != "hello" {
		t.Fatalf("got %q, want hello", got)
	}
}
