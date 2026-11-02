package deletefile

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

// A file inside the workspace goes, and the acknowledgement names where it was.
func TestDeletesAFileInTheWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("bye"), 0o644); err != nil {
		t.Fatalf("could not set up: %v", err)
	}

	p := New(tools.New(), workspace.MustNew(root))

	got, err := runTool(t, p, "notes.txt")
	if err != nil {
		t.Fatalf("deleting a relative path failed: %v", err)
	}
	if got == "" {
		t.Fatal("nothing was reported back")
	}
	if _, err := os.Stat(filepath.Join(root, "notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("the file is still there: %v", err)
	}
}

// The path deleted must be the one that was judged, even when a file of the same
// name is sitting next to the process.
func TestARelativePathDeletesTheWorkspacesFile(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "same.txt"), []byte("workspace"), 0o644); err != nil {
		t.Fatalf("could not set up: %v", err)
	}
	if err := os.WriteFile(filepath.Join(elsewhere, "same.txt"), []byte("next to the process"), 0o644); err != nil {
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
	if _, err := runTool(t, p, "same.txt"); err != nil {
		t.Fatalf("deleting failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "same.txt")); !os.IsNotExist(err) {
		t.Fatal("the workspace's file is still there")
	}
	if _, err := os.Stat(filepath.Join(elsewhere, "same.txt")); err != nil {
		t.Fatal("the file next to the process was deleted instead — the path judged and the path deleted must match")
	}
}

// Outside the workspace is refused, and nothing is removed.
func TestDeletingOutsideTheWorkspaceIsRefused(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere.txt")
	if err := os.WriteFile(outside, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("could not set up: %v", err)
	}

	p := New(tools.New(), workspace.MustNew(root))

	if _, err := runTool(t, p, outside); err == nil {
		t.Fatal("a file outside the workspace was deleted")
	}
	if _, err := runTool(t, p, "../escape.txt"); err == nil {
		t.Fatal("a path climbing out of the workspace was deleted")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("the file outside was removed anyway: %v", err)
	}
}

// A folder is refused: removing one takes everything inside it, which is a
// different decision.
func TestAFolderIsRefused(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "folder"), 0o755); err != nil {
		t.Fatalf("could not set up: %v", err)
	}

	p := New(tools.New(), workspace.MustNew(root))

	if _, err := runTool(t, p, "folder"); err == nil {
		t.Fatal("a folder was removed")
	}
	if _, err := os.Stat(filepath.Join(root, "folder")); err != nil {
		t.Fatalf("the folder was removed anyway: %v", err)
	}
}

// A file that is not there is an error, not a silent success: "deleted" for
// something that never existed is a lie the model would repeat to the person.
func TestAMissingFileIsAnError(t *testing.T) {
	p := New(tools.New(), workspace.MustNew(t.TempDir()))

	if _, err := runTool(t, p, "nothing-here.txt"); err == nil {
		t.Fatal("deleting a file that does not exist reported success")
	}
}
