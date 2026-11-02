package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A relative path belongs to the workspace, not to wherever the harness was
// started. This is the bug that made every relative path read as "outside the
// workspace": the answer depended on the process's working directory, which is
// invisible to the agent and to the person reading the error.
func TestARelativePathIsResolvedAgainstTheWorkspace(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()

	// Be somewhere else entirely, the way the harness is when it is launched
	// from its own source tree.
	was, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not read the working directory: %v", err)
	}
	if err := os.Chdir(other); err != nil {
		t.Fatalf("could not move to another directory: %v", err)
	}
	defer os.Chdir(was)

	r := MustNew(root)

	if _, ok := r.Allow("notes.txt"); !ok {
		t.Fatalf("a plain relative path was refused while working from %s", other)
	}
	if _, ok := r.Allow("a/b/c.txt"); !ok {
		t.Fatal("a relative path with folders in it was refused")
	}
}

// The same relative path means the same thing from anywhere, which is the point
// of anchoring it.
func TestARelativePathMeansTheSameFromAnywhere(t *testing.T) {
	root := t.TempDir()
	r := MustNew(root)

	here, _ := os.Getwd()
	defer os.Chdir(here)

	answers := make([]bool, 0, 3)
	for _, where := range []string{root, os.TempDir(), here} {
		if err := os.Chdir(where); err != nil {
			t.Fatalf("could not move to %s: %v", where, err)
		}
		_, ok := r.Allow("notes.txt")
		answers = append(answers, ok)
	}

	for _, ok := range answers {
		if ok != answers[0] {
			t.Fatalf("working from a different directory changed the answer (%v)", answers)
		}
	}
	if !answers[0] {
		t.Fatal("a relative path was refused from every directory")
	}
}

// Climbing out with .. is still refused, so anchoring cannot be used to escape.
func TestClimbingOutOfTheWorkspaceIsRefused(t *testing.T) {
	root := t.TempDir()
	r := MustNew(root)

	if _, ok := r.Allow("../elsewhere.txt"); ok {
		t.Fatal("../elsewhere.txt was allowed out of the workspace")
	}
	if _, ok := r.Allow("nested/../../out.txt"); ok {
		t.Fatal("a path climbing out through a folder was allowed")
	}
}

// An absolute path outside the roots is refused whatever else happens.
func TestAnAbsolutePathOutsideIsRefused(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere.txt")
	r := MustNew(root)

	if _, ok := r.Allow(outside); ok {
		t.Fatalf("%s was allowed, but it is not in %s", outside, root)
	}
}

// Naming the workspace itself is allowed: you may work on the folder.
func TestTheWorkspaceItselfIsAllowed(t *testing.T) {
	root := t.TempDir()
	r := MustNew(root)

	if _, ok := r.Allow(root); !ok {
		t.Fatal("the workspace's own folder was refused")
	}
	if _, ok := r.Allow("."); !ok {
		t.Fatal("a relative path of . was refused")
	}
}

// An empty path is refused rather than quietly meaning the workspace.
func TestAnEmptyPathIsRefused(t *testing.T) {
	r := MustNew(t.TempDir())

	if _, ok := r.Allow(""); ok {
		t.Fatal("an empty path was allowed")
	}
	if _, ok := r.Allow("   "); ok {
		t.Fatal("a path of spaces was allowed")
	}
}

// The refusal still names where the agent may go, because that is the whole use
// of the message. Compared against the resolved root: on Windows a temp directory
// can come back in the short 8.3 form while resolution returns the long one, so
// the raw directory would never match.
func TestARefusalNamesTheRoots(t *testing.T) {
	root := t.TempDir()
	r := MustNew(root)

	err := r.Check(filepath.Join(t.TempDir(), "elsewhere.txt"))
	if err == nil {
		t.Fatal("an outside path was not refused")
	}
	if !strings.Contains(err.Error(), r.Dirs()[0]) {
		t.Fatalf("the refusal does not name the workspace: %v", err)
	}
}

// A path starting with a separator means somewhere else, not somewhere in the
// workspace. On Windows a leading slash is rooted but not absolute, so reading it
// as relative would pull /etc/passwd inside the workspace instead of refusing it.
func TestALeadingSeparatorMeansSomewhereElse(t *testing.T) {
	r := MustNew(t.TempDir())

	if _, ok := r.Allow("/etc/passwd"); ok {
		t.Fatal("/etc/passwd was allowed: a leading slash must never read as relative")
	}
}

// What a tool is told to open must be the file that was judged. With a file of
// the same name sitting next to the process, resolving against the process
// instead of the workspace would open the wrong one — which is precisely how the
// file tools came to read files outside the workspace.
func TestResolveGivesTheFileInsideTheWorkspace(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "same.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatalf("could not set up: %v", err)
	}
	if err := os.WriteFile(filepath.Join(elsewhere, "same.txt"), []byte("outside"), 0o644); err != nil {
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

	r := MustNew(root)
	got, err := r.Resolve("same.txt")
	if err != nil {
		t.Fatalf("a relative path was refused: %v", err)
	}

	want := filepath.Join(r.Dirs()[0], "same.txt")
	if got != want {
		t.Fatalf("resolved to %s, want %s — the file judged is the file that must be opened", got, want)
	}

	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("could not read what was resolved: %v", err)
	}
	if string(data) != "inside" {
		t.Fatalf("opened the file next to the process: %q", data)
	}
}

// Resolving refuses everything the check refuses, or the two would disagree.
func TestResolveRefusesWhatIsOutside(t *testing.T) {
	r := MustNew(t.TempDir())

	for _, path := range []string{
		filepath.Join(t.TempDir(), "x.txt"),
		"/etc/passwd",
		"../out.txt",
		"nested/../../out.txt",
	} {
		if _, err := r.Resolve(path); err == nil {
			t.Errorf("%s resolved, but it is outside the workspace", path)
		}
	}
}
