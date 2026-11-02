package readfile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/usually-broken/lola/core/tools"
	"github.com/usually-broken/lola/core/workspace"
)

func runTool(t *testing.T, p *Plugin, path string) (string, error) {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("could not build arguments: %v", err)
	}
	return p.Run(context.Background(), raw)
}

// The end of the hole: with a file of the same name sitting next to the process,
// a relative path must still read the one inside the workspace. Before the fix,
// this call was judged against the workspace and then opened against the process's
// working directory, so it read the file outside.
func TestARelativePathReadsTheWorkspacesFile(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("the workspace's notes"), 0o644); err != nil {
		t.Fatalf("could not set up: %v", err)
	}
	if err := os.WriteFile(filepath.Join(elsewhere, "notes.txt"), []byte("notes from somewhere else"), 0o644); err != nil {
		t.Fatalf("could not set up: %v", err)
	}

	was, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not read the working directory: %v", err)
	}
	if err := os.Chdir(elsewhere); err != nil {
		t.Fatalf("could not move: %v", err)
	}
	defer os.Chdir(was)

	p := New(tools.New(), workspace.MustNew(root))

	got, err := runTool(t, p, "notes.txt")
	if err != nil {
		t.Fatalf("reading a relative path failed: %v", err)
	}
	if got != "the workspace's notes" {
		t.Fatalf("read %q, want the workspace's file", got)
	}
}

// A path that climbs out is still refused, and the refusal names the workspace.
func TestReadingOutsideTheWorkspaceIsRefused(t *testing.T) {
	root := t.TempDir()
	p := New(tools.New(), workspace.MustNew(root))

	if _, err := runTool(t, p, filepath.Join(t.TempDir(), "elsewhere.txt")); err == nil {
		t.Fatal("a path outside the workspace was read")
	}
	if _, err := runTool(t, p, "../escape.txt"); err == nil {
		t.Fatal("a path climbing out of the workspace was read")
	}
}
