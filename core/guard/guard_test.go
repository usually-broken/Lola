package guard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func args(t *testing.T, m map[string]string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSessionsAreDenied(t *testing.T) {
	home := t.TempDir()
	r := Default(home, "")

	decision := r.Check("write_file", args(t, map[string]string{
		"path": filepath.Join(home, "sessions", "abc.json"),
	}))
	if decision != Deny {
		t.Fatalf("writing a session should be denied, got %s", decision)
	}
}

func TestProfilesAreDenied(t *testing.T) {
	home := t.TempDir()
	r := Default(home, "")

	decision := r.Check("write_file", args(t, map[string]string{
		"path": filepath.Join(home, "profiles", "default.json"),
	}))
	if decision != Deny {
		t.Fatalf("writing a profile should be denied, got %s", decision)
	}
}

func TestConfigNeedsApproval(t *testing.T) {
	home := t.TempDir()
	r := Default(home, "")

	decision := r.Check("write_file", args(t, map[string]string{
		"path": filepath.Join(home, "lola.yaml"),
	}))
	if decision != Ask {
		t.Fatalf("editing the config should ask first, got %s", decision)
	}
}

func TestPluginsNeedApproval(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	r := Default(home, "")
	r.Source = src

	decision := r.Check("write_file", args(t, map[string]string{
		"path": filepath.Join(src, "plugins", "greet", "greet.go"),
	}))
	if decision != Ask {
		t.Fatalf("editing a plugin should ask first, got %s", decision)
	}
}

func TestOrdinaryFileIsAllowed(t *testing.T) {
	home := t.TempDir()
	r := Default(home, "")

	decision := r.Check("write_file", args(t, map[string]string{
		"path": filepath.Join(home, "workspace", "note.txt"),
	}))
	if decision != Allow {
		t.Fatalf("an ordinary file should be allowed, got %s", decision)
	}
}

func TestSourceRuleDisarmedWhenUnset(t *testing.T) {
	home := t.TempDir()
	src := t.TempDir()
	r := Default(home, "") // Source left empty

	decision := r.Check("write_file", args(t, map[string]string{
		"path": filepath.Join(src, "main.go"),
	}))
	if decision != Allow {
		t.Fatalf("with no source configured the rule should not arm, got %s", decision)
	}
}

func TestDangerousShellCommandNeedsApproval(t *testing.T) {
	r := Default(t.TempDir(), "")

	for _, cmd := range []string{"rm -rf /tmp/x", "sudo rm foo", "dd if=/dev/zero of=/dev/disk"} {
		if d := r.Check("run_shell", args(t, map[string]string{"command": cmd})); d != Ask {
			t.Errorf("%q should ask first, got %s", cmd, d)
		}
	}
}

func TestOrdinaryShellCommandAllowed(t *testing.T) {
	r := Default(t.TempDir(), "")

	for _, cmd := range []string{"ls -la", "go build ./...", "date", "cat README.md"} {
		if d := r.Check("run_shell", args(t, map[string]string{"command": cmd})); d != Allow {
			t.Errorf("%q should be allowed, got %s", cmd, d)
		}
	}
}

func TestProtectedBeatsGated(t *testing.T) {
	home := t.TempDir()
	r := Default(home, "")
	r.Gated = append(r.Gated, filepath.Join(home, "sessions"))

	// Even if someone misconfigures sessions as gated, denied wins.
	d := r.Check("write_file", args(t, map[string]string{
		"path": filepath.Join(home, "sessions", "x.json"),
	}))
	if d != Deny {
		t.Fatalf("protected must win over gated, got %s", d)
	}
}

func TestSymlinkCannotDisguiseTarget(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	r := Default(home, "")

	link := filepath.Join(home, "link-to-outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	// A path through the link points at `outside`, which is not protected or
	// gated, so it is allowed — but crucially, a link pointing AT sessions is
	// denied.
	link2 := filepath.Join(home, "sneaky-sessions")
	if err := os.MkdirAll(filepath.Join(home, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "sessions"), link2); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	d := r.Check("write_file", args(t, map[string]string{
		"path": filepath.Join(link2, "abc.json"),
	}))
	if d != Deny {
		t.Fatalf("a symlink into sessions must still be denied, got %s", d)
	}
}

func TestRefuseAllApprover(t *testing.T) {
	var a Approver = RefuseAll{}
	if got := a.Approve(nil, "anything"); got != Unavailable {
		t.Fatalf("RefuseAll should return Unavailable, got %v", got)
	}
}

func TestDescribeIsReadable(t *testing.T) {
	got := Describe("write_file", args(t, map[string]string{"path": "/tmp/x.txt"}))
	if got != "write to /tmp/x.txt" {
		t.Fatalf("Describe = %q", got)
	}
	got = Describe("run_shell", args(t, map[string]string{"command": "rm -rf /"}))
	if got != "run: rm -rf /" {
		t.Fatalf("Describe = %q", got)
	}
}
