package home

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirDefaultsToDotLola(t *testing.T) {
	t.Setenv(EnvVar, "")
	dir := Dir()
	if !strings.HasSuffix(dir, DirName) {
		t.Fatalf("Dir() = %q, want it to end in %q", dir, DirName)
	}
}

func TestDirHonoursEnvVar(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom-lola")
	t.Setenv(EnvVar, want)
	if got := Dir(); got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}
}

func TestPathsLiveUnderHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "h")
	t.Setenv(EnvVar, home)

	for _, p := range []string{ConfigPath(), SessionsDir(), BinDir(), BinaryPath()} {
		if !strings.HasPrefix(p, home) {
			t.Errorf("%q is not under the home directory %q", p, home)
		}
	}
	if ConfigPath() != filepath.Join(home, "lola.yaml") {
		t.Errorf("ConfigPath() = %q", ConfigPath())
	}
	if SessionsDir() != filepath.Join(home, "sessions") {
		t.Errorf("SessionsDir() = %q", SessionsDir())
	}
}

func TestEnsureCreatesHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "made", "lola")
	t.Setenv(EnvVar, home)

	if err := Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	info, err := os.Stat(home)
	if err != nil || !info.IsDir() {
		t.Fatalf("home directory was not created: %v", err)
	}
}
