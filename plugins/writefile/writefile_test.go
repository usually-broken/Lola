package writefile

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

func TestRunWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "out.txt")
	p := New(tools.New(), nil)

	if _, err := p.Run(context.Background(), jsonArgs(t, map[string]string{"path": path, "content": "written"})); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(data) != "written" {
		t.Fatalf("content = %q, want %q", string(data), "written")
	}
}

func TestRunMissingPathIsAnError(t *testing.T) {
	p := New(tools.New(), nil)
	if _, err := p.Run(context.Background(), json.RawMessage(`{"content":"x"}`)); err == nil {
		t.Fatal("expected an error when path is missing")
	}
}

func TestRunRefusesPathOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	ws := workspace.MustNew(root)
	p := New(tools.New(), ws)

	outside := filepath.Join(t.TempDir(), "out.txt")
	_, err := p.Run(context.Background(), jsonArgs(t, map[string]string{"path": outside, "content": "x"}))
	if err == nil {
		t.Fatal("writing outside the workspace should be refused")
	}
	if !strings.Contains(err.Error(), "outside the workspace") {
		t.Fatalf("error should explain the refusal, got: %v", err)
	}
}

func TestRunAllowsWriteInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	ws := workspace.MustNew(root)
	p := New(tools.New(), ws)

	inside := filepath.Join(root, "sub", "ok.txt")
	if _, err := p.Run(context.Background(), jsonArgs(t, map[string]string{"path": inside, "content": "fine"})); err != nil {
		t.Fatalf("writing inside the workspace should work: %v", err)
	}
	data, err := os.ReadFile(inside)
	if err != nil || string(data) != "fine" {
		t.Fatalf("file not written correctly: %v", err)
	}
}
