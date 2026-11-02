package catalog_test

import (
	"path/filepath"
	"testing"

	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/registry/config"
)

// Every name the shipped config asks for has to be in the catalog.
//
// The catalog is what makes a plugin name real: a config entry with no import
// compiles happily and then refuses to start, which is a failure that reaches a
// person rather than a test. This is that test.
func TestShippedConfigOnlyNamesPluginsWeHave(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "lola.yaml"))
	if err != nil {
		t.Fatalf("reading the shipped config: %v", err)
	}

	have := map[string]bool{}
	for _, name := range plugin.Names() {
		have[name] = true
	}
	if len(have) == 0 {
		t.Fatal("the catalog is empty: nothing imported it")
	}

	for _, name := range cfg.Plugins {
		if !have[name] {
			t.Errorf("lola.yaml mounts %q, but nothing in the catalog registers it", name)
		}
	}
}

// And the other way round: a plugin in the catalog that the shipped config does
// not mount is either a mistake or a plugin nobody can reach without editing yaml.
// Some are deliberately off by default, so this one names them rather than failing
// silently — the list should be short and every entry should be a decision.
func TestShippedConfigMountsWhatTheCatalogOffers(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "lola.yaml"))
	if err != nil {
		t.Fatalf("reading the shipped config: %v", err)
	}

	mounted := map[string]bool{}
	for _, name := range cfg.Plugins {
		mounted[name] = true
	}

	deliberatelyOff := map[string]bool{
		"promptdemo": false, // deleted; kept here only if it ever comes back
	}

	for _, name := range plugin.Names() {
		if mounted[name] || deliberatelyOff[name] {
			continue
		}
		t.Logf("in the catalog but not in lola.yaml: %s", name)
	}
}
