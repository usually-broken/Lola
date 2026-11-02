// Package workspace is the workspace seam: the roots an agent's file tools are
// allowed to touch.
//
// A file tool asks the workspace whether a path is inside its roots. Inside,
// it may proceed; outside, it is refused. The roots are configuration, not
// code, so the same binary can be boxed into its own folder or pointed at a
// real project.
//
// This bounds the file tools only. A shell command can reach anywhere a shell
// can reach, so it is governed by approval instead — a separate concern.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Roots is the set of directories an agent may touch. Reading is safe while
// another goroutine changes it, since the agent's tools check the roots from a
// different goroutine than the interface that sets them.
type Roots struct {
	mu   sync.RWMutex
	dirs []string
}

// None returns a Roots that permits nothing. Used when no workspace is
// configured and strictness is wanted.
func None() *Roots { return &Roots{} }

// New returns Roots from a list of directories. Relative paths are resolved
// against the current directory; "~" is expanded to the home directory. Empty
// entries are ignored. Duplicates are collapsed.
func New(dirs ...string) (*Roots, error) {
	r := &Roots{}
	seen := map[string]bool{}
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		resolved, err := resolve(d)
		if err != nil {
			return nil, err
		}
		if !seen[resolved] {
			seen[resolved] = true
			r.dirs = append(r.dirs, resolved)
		}
	}
	return r, nil
}

// MustNew is New, panicking on error. For tests and fixed configuration.
func MustNew(dirs ...string) *Roots {
	r, err := New(dirs...)
	if err != nil {
		panic(err)
	}
	return r
}

// Dirs returns the resolved root directories.
func (r *Roots) Dirs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.dirs...)
}

// Empty reports whether there are no roots at all.
func (r *Roots) Empty() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.dirs) == 0
}

// Set replaces the roots at runtime. The file tools read the current roots on
// every call, so a change takes effect at once, with no restart.
func (r *Roots) Set(dirs ...string) error {
	next, err := New(dirs...)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dirs = next.dirs
	return nil
}

// Allow reports whether path is inside one of the roots, returning the root it
// belongs to. A path equal to a root is allowed (you may work on the folder
// itself). Symlinks are resolved before the check, so a link cannot be used to
// slip outside.
//
// A relative path is resolved against the first root rather than against the
// directory the harness happens to have been started in. The workspace is where
// an agent's world starts, so "notes.txt" means <workspace>/notes.txt — and it
// means that whether the harness was launched from the workspace, from its own
// source tree, or from anywhere else. Resolving against the process instead made
// every relative path depend on how the program was started, which is not
// something an agent can see or reason about.
func (r *Roots) Allow(path string) (string, bool) {
	r.mu.RLock()
	dirs := append([]string(nil), r.dirs...)
	r.mu.RUnlock()

	if len(dirs) == 0 {
		return "", false
	}
	if strings.TrimSpace(path) == "" {
		return "", false
	}

	target, err := realPath(anchor(path, dirs[0]))
	if err != nil {
		return "", false
	}

	for _, root := range dirs {
		if within(target, root) {
			return root, true
		}
	}
	return "", false
}

// anchor resolves a relative path against root. A path that names a place
// outright is left alone: the roots still decide whether it may be touched.
//
// "Names a place outright" is not the same as filepath.IsAbs. On Windows a path
// starting with a separator is rooted but not absolute — IsAbs("/etc/passwd") is
// false there — so treating it as relative would join it onto the workspace and
// allow it. A leading slash means somewhere else on every platform, and it must
// keep meaning that.
func anchor(path, root string) string {
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`) {
		return path
	}
	return filepath.Join(root, path)
}

// Resolve returns the path a tool should actually use, refusing anything outside
// the roots. The returned path is absolute and symlink-free.
//
// A tool must open the path this returns, not the one it was handed. Checking one
// path and opening another is exactly how a boundary gets a hole in it: with a
// relative path, the check resolves against the workspace while an ordinary
// os.Open resolves against the process's working directory, so a call could be
// judged inside the workspace and then read a file outside it.
func (r *Roots) Resolve(path string) (string, error) {
	dirs := r.Dirs()
	if len(dirs) == 0 {
		return "", r.refusal(path)
	}
	if _, ok := r.Allow(path); !ok {
		return "", r.refusal(path)
	}
	return realPath(anchor(path, dirs[0]))
}

// Check returns an error naming the roots when path is not allowed. Tools call
// it and return the error to the model, which is how a refused write reads.
func (r *Roots) Check(path string) error {
	if _, ok := r.Allow(path); ok {
		return nil
	}
	return r.refusal(path)
}

// refusal says why a path cannot be touched, naming the roots so the message is
// usable for whoever has to fix it.
func (r *Roots) refusal(path string) error {
	dirs := r.Dirs()
	if len(dirs) == 0 {
		return fmt.Errorf("workspace: no workspace is configured, so %q cannot be touched", path)
	}
	return fmt.Errorf("workspace: %q is outside the workspace (%s)", path, strings.Join(dirs, ", "))
}

// within reports whether target is root or inside root.
func within(target, root string) bool {
	if target == root {
		return true
	}
	return strings.HasPrefix(target, root+string(os.PathSeparator))
}

// realPath resolves a path to an absolute, symlink-free form. For a path that
// does not exist yet, the deepest existing ancestor is resolved and the
// remaining components are appended, so writes to new files are judged
// correctly rather than failing.
func realPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	// Walk up until we reach something that exists, remembering the parts we
	// removed so they can be put back.
	existing := abs
	var missing []string
	for {
		if resolved, err := filepath.EvalSymlinks(existing); err == nil {
			parts := append([]string{resolved}, missing...)
			return filepath.Join(parts...), nil
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return abs, nil // reached the filesystem root; use the absolute path
		}
		missing = append([]string{filepath.Base(existing)}, missing...)
		existing = parent
	}
}

// resolve turns a configured directory into an absolute, cleaned path with any
// symlinks resolved, so that comparing it with a resolved target is exact.
// macOS makes this necessary: /var is a symlink to /private/var, so a root left
// unresolved would never match a resolved path inside it.
func resolve(dir string) (string, error) {
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("workspace: resolve home: %w", err)
		}
		dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("workspace: resolve %s: %w", dir, err)
	}
	abs = filepath.Clean(abs)

	// Resolve symlinks as far as the path exists, so root and target are
	// compared in the same form.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	// The directory does not exist yet: resolve its nearest existing ancestor
	// and re-append the rest.
	existing := abs
	var missing []string
	for {
		if resolved, err := filepath.EvalSymlinks(existing); err == nil {
			return filepath.Join(append([]string{resolved}, missing...)...), nil
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return abs, nil
		}
		missing = append([]string{filepath.Base(existing)}, missing...)
		existing = parent
	}
}
