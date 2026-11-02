package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllowInsideRoot(t *testing.T) {
	root := t.TempDir()
	r := MustNew(root)

	if _, ok := r.Allow(filepath.Join(root, "a.txt")); !ok {
		t.Fatalf("%s/a.txt should be allowed", root)
	}
	if _, ok := r.Allow(root); !ok {
		t.Fatal("the root itself should be allowed")
	}
}

func TestRefusesOutsideRoot(t *testing.T) {
	root := t.TempDir()
	r := MustNew(root)

	if _, ok := r.Allow("/etc/passwd"); ok {
		t.Fatal("/etc/passwd should be refused")
	}
	if _, ok := r.Allow(filepath.Join(t.TempDir(), "elsewhere.txt")); ok {
		t.Fatal("a path in another temp dir should be refused")
	}
}

func TestRefusesParentEscape(t *testing.T) {
	root := t.TempDir()
	r := MustNew(root)

	escape := filepath.Join(root, "..", "outside.txt")
	if _, ok := r.Allow(escape); ok {
		t.Fatalf("%s should be refused", escape)
	}
}

func TestMultipleRoots(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	r := MustNew(a, b)

	if _, ok := r.Allow(filepath.Join(a, "x")); !ok {
		t.Fatal("path in first root should be allowed")
	}
	if _, ok := r.Allow(filepath.Join(b, "y")); !ok {
		t.Fatal("path in second root should be allowed")
	}
	if _, ok := r.Allow(t.TempDir()); ok {
		t.Fatal("a path in neither root should be refused")
	}
}

func TestEmptyRootsAllowNothing(t *testing.T) {
	r := None()
	if _, ok := r.Allow("/anything"); ok {
		t.Fatal("an empty workspace should allow nothing")
	}
	if !r.Empty() {
		t.Fatal("None() should report Empty")
	}
}

func TestNewFileUnderRootIsAllowed(t *testing.T) {
	root := t.TempDir()
	r := MustNew(root)

	// The file does not exist yet; a write to it must still be judged inside.
	newFile := filepath.Join(root, "sub", "new.txt")
	if _, ok := r.Allow(newFile); !ok {
		t.Fatalf("a not-yet-created file under the root should be allowed: %s", newFile)
	}
}

func TestSymlinkCannotEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	// A symlink inside the root pointing outside it.
	link := filepath.Join(root, "sneaky")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	r := MustNew(root)
	target := filepath.Join(link, "file.txt")
	if _, ok := r.Allow(target); ok {
		t.Fatalf("a path through an escaping symlink must be refused: %s", target)
	}
}

func TestCheckNamesTheRoots(t *testing.T) {
	root := t.TempDir()
	r := MustNew(root)

	err := r.Check("/etc/passwd")
	if err == nil {
		t.Fatal("Check should fail for a path outside the root")
	}
	// Compare against the resolved root: on Windows t.TempDir can hand back a
	// short 8.3 path while resolution returns the long form, so the raw
	// directory would never match.
	if !strings.Contains(err.Error(), r.Dirs()[0]) {
		t.Fatalf("error should name the allowed root, got: %v", err)
	}
}

func TestTildeExpands(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	r := MustNew("~/")
	if len(r.Dirs()) != 1 || r.Dirs()[0] != home {
		t.Fatalf("Dirs() = %v, want [%s]", r.Dirs(), home)
	}
}

func TestDuplicatesCollapsed(t *testing.T) {
	root := t.TempDir()
	r := MustNew(root, root, root)
	if len(r.Dirs()) != 1 {
		t.Fatalf("Dirs() = %v, want one entry", r.Dirs())
	}
}
