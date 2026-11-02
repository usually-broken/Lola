package keepfile

import (
	"os"
	"path/filepath"
	"testing"
)

// open builds a plugin over a file, the way the tree does.
func open(t *testing.T, path string) *Plugin {
	t.Helper()

	p := &Plugin{path: path}
	if _, err := p.Setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return p
}

// A value kept in one run is there in the next one, in the same scope.
func TestAValueSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")

	first := open(t, path)
	if err := first.Set(ScopeGlobal, "mode", "ask"); err != nil {
		t.Fatalf("keeping a value: %v", err)
	}

	second := open(t, path)
	if got, ok := second.Get(ScopeGlobal, "mode"); !ok || got != "ask" {
		t.Errorf("after a restart: %q %v, want ask true", got, ok)
	}
}

// Scopes do not leak: a value kept for one session is not the default, and not
// another session's either.
func TestScopesAreKeptApart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	p := open(t, path)

	if err := p.Set(ScopeSession+"abc", "model", "one"); err != nil {
		t.Fatal(err)
	}
	if err := p.Set(ScopeSession+"def", "model", "two"); err != nil {
		t.Fatal(err)
	}

	if got, _ := p.Get(ScopeSession+"abc", "model"); got != "one" {
		t.Errorf("session abc = %q, want one", got)
	}
	if got, _ := p.Get(ScopeSession+"def", "model"); got != "two" {
		t.Errorf("session def = %q, want two", got)
	}
	if _, ok := p.Get(ScopeGlobal, "model"); ok {
		t.Error("a session's value turned up as the global default")
	}
}

// Nothing kept is a miss, not an error and not an empty string pretending to be
// a value.
func TestNothingKeptIsAMiss(t *testing.T) {
	p := open(t, filepath.Join(t.TempDir(), "settings.json"))

	if got, ok := p.Get(ScopeGlobal, "mode"); ok {
		t.Errorf("got %q and true from an empty file, want a miss", got)
	}
}

// The file before scopes existed was a flat map. Nobody should lose the settings
// they already made, so it is read into the global scope.
func TestAFlatFileIsMovedIntoTheGlobalScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"model":"gemma4:cloud","mode":"ask"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	p := open(t, path)
	if got, ok := p.Get(ScopeGlobal, "mode"); !ok || got != "ask" {
		t.Errorf("old mode = %q %v, want ask true", got, ok)
	}
	if got, ok := p.Get(ScopeGlobal, "model"); !ok || got != "gemma4:cloud" {
		t.Errorf("old model = %q %v, want gemma4:cloud true", got, ok)
	}
}

// A corrupt file is ignored rather than fatal, and the next choice writes a good
// one over the top.
func TestACorruptFileIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := open(t, path)
	if _, ok := p.Get(ScopeGlobal, "mode"); ok {
		t.Error("a corrupt file produced a value")
	}
	if err := p.Set(ScopeGlobal, "mode", "workspace"); err != nil {
		t.Fatalf("writing over a corrupt file: %v", err)
	}
	if got, _ := open(t, path).Get(ScopeGlobal, "mode"); got != "workspace" {
		t.Errorf("after rewriting = %q, want workspace", got)
	}
}
