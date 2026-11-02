package config

import (
	"reflect"
	"testing"
)

func TestParseGuardSection(t *testing.T) {
	cfg, err := Parse([]byte(`
guard:
  protected:
    - /home/me/.lola/sessions
    - /home/me/.lola/profiles
  gated:
    - /home/me/.lola/lola.yaml
  dangerous:
    - rm
    - sudo
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if want := []string{"/home/me/.lola/sessions", "/home/me/.lola/profiles"}; !reflect.DeepEqual(cfg.Guard.Protected, want) {
		t.Errorf("protected = %v, want %v", cfg.Guard.Protected, want)
	}
	if want := []string{"/home/me/.lola/lola.yaml"}; !reflect.DeepEqual(cfg.Guard.Gated, want) {
		t.Errorf("gated = %v, want %v", cfg.Guard.Gated, want)
	}
	if want := []string{"rm", "sudo"}; !reflect.DeepEqual(cfg.Guard.Dangerous, want) {
		t.Errorf("dangerous = %v, want %v", cfg.Guard.Dangerous, want)
	}
}

func TestParseGuardAlongsidePluginsAndWorkspace(t *testing.T) {
	cfg, err := Parse([]byte(`
plugins:
  - greet

workspace:
  - ~/.lola/workspace

guard:
  dangerous:
    - rm
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Plugins) != 1 || cfg.Plugins[0] != "greet" {
		t.Errorf("plugins = %v", cfg.Plugins)
	}
	if len(cfg.Workspace) != 1 {
		t.Errorf("workspace = %v", cfg.Workspace)
	}
	if len(cfg.Guard.Dangerous) != 1 || cfg.Guard.Dangerous[0] != "rm" {
		t.Errorf("dangerous = %v", cfg.Guard.Dangerous)
	}
}

func TestParseGuardSubListOutsideGuardIsAnError(t *testing.T) {
	if _, err := Parse([]byte("protected:\n  - /x\n")); err == nil {
		t.Fatal("a guard sub-list outside a guard: block should be an error")
	}
}

func TestParseListDirectlyUnderGuardIsAnError(t *testing.T) {
	if _, err := Parse([]byte("guard:\n  - /x\n")); err == nil {
		t.Fatal("a list item directly under guard: should be an error")
	}
}
