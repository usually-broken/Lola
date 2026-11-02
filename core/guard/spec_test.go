package guard

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestFromSpecWithEmptyListsKeepsDefaults(t *testing.T) {
	home := t.TempDir()
	r := FromSpec(home, "", Spec{})

	if len(r.Protected) == 0 {
		t.Error("an empty spec should keep the default protected list")
	}
	if len(r.Dangerous) == 0 {
		t.Error("an empty spec should keep the default dangerous list")
	}
}

func TestFromSpecReplacesLists(t *testing.T) {
	home := t.TempDir()
	r := FromSpec(home, "", Spec{
		Protected: []string{"/custom/protected"},
		Dangerous: []string{"shred-only"},
	})

	if len(r.Protected) != 1 || r.Protected[0] != "/custom/protected" {
		t.Errorf("protected = %v, want the spec's list", r.Protected)
	}
	if len(r.Dangerous) != 1 || r.Dangerous[0] != "shred-only" {
		t.Errorf("dangerous = %v, want the spec's list", r.Dangerous)
	}
}

func TestFromSpecKeepsTheSourceGated(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()

	// The spec lists its own gated paths; the source must still be gated.
	r := FromSpec(home, src, Spec{Gated: []string{"/only/this"}})

	args, _ := json.Marshal(map[string]string{"path": filepath.Join(src, "main.go")})
	if got := r.Check("write_file", args); got != Ask {
		t.Fatalf("editing the source should need approval, got %s", got)
	}
}

func TestSetSourceChangesTheRule(t *testing.T) {
	home := t.TempDir()
	r := FromSpec(home, "", Spec{})

	if r.SourceOf() != "" {
		t.Fatal("no source should be set initially")
	}

	src := t.TempDir()
	r.SetSource(src)

	args, _ := json.Marshal(map[string]string{"path": filepath.Join(src, "x.go")})
	if got := r.Check("write_file", args); got != Ask {
		t.Fatalf("after SetSource, editing it should need approval, got %s", got)
	}

	r.SetSource("")
	if got := r.Check("write_file", args); got != Allow {
		t.Fatalf("after clearing the source it should be allowed, got %s", got)
	}
}

func TestFromSpecRulesStillProtect(t *testing.T) {
	home := t.TempDir()
	r := FromSpec(home, "", Spec{
		Protected: []string{"/somewhere/sealed"},
	})

	if got := r.Check("write_file", []byte(`{"path":"/somewhere/sealed/x.txt"}`)); got != Deny {
		t.Fatalf("a spec-protected path should be denied, got %s", got)
	}
}
