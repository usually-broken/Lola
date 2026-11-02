// Package config reads lola.yaml and builds the plugin tree it describes.
//
// This is the piece that removes the privileged core: main no longer names the
// plugins, the config file does. Adding a plugin is a line in the file.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/home"
	"github.com/usually-broken/lola/core/plugin"
)

// DefaultPath is the config file the harness looks for.
const DefaultPath = "lola.yaml"

// Config is the parsed contents of lola.yaml.
type Config struct {
	// Plugins is the list of plugin names to mount, in mount order.
	Plugins []string

	// Workspace is the list of directories an agent's file tools may touch.
	// Empty means no workspace is configured.
	Workspace []string

	// Guard is the guard policy, if the config supplies one.
	Guard GuardSpec

	// Providers is where model calls can go, in the order listed. The first is
	// the one the harness starts on.
	Providers []ProviderSpec
}

// ProviderSpec is one entry in the providers list: an endpoint, a name for it,
// and where its key comes from if it needs one at all.
type ProviderSpec struct {
	// Name is what a person sees: "local", "openrouter".
	Name string

	// URL is the endpoint root, without /chat/completions.
	URL string

	// Kind is the wire format this endpoint speaks. Empty means "openai", which
	// is what a config written before there was a choice means.
	Kind string

	// Key is either the name of an environment variable to read the key from,
	// or the key itself. The variable name is the better habit: then there is
	// nothing secret in this file, and nothing secret can end up in a commit.
	Key string
}

// GuardSpec is the guard policy a config can set. Each list, when non-empty,
// replaces the corresponding built-in default.
type GuardSpec struct {
	Protected []string
	Gated     []string
	Dangerous []string
}

// Load reads and parses the config at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	return Parse(data)
}

// LoadResolved loads the config, searching sensible places when path is the
// default name and the file is not found where asked:
//
//  1. the path as given (so ./lola.yaml wins if you are in the repo)
//  2. the Lola home directory (default ~/.lola), so an installed lola finds
//     the config that lives with it
//
// An explicit path that does not exist is an error, not a search.
func LoadResolved(path string) (*Config, error) {
	if data, err := os.ReadFile(path); err == nil {
		return Parse(data)
	}

	if path == DefaultPath {
		if data, err := os.ReadFile(home.ConfigPath()); err == nil {
			return Parse(data)
		}
	}

	return nil, fmt.Errorf("config: read %s: no such file (looked in the current directory and in %s)", path, home.Dir())
}

// Parse reads a Config from raw YAML.
//
// The format is deliberately tiny — a `plugins:` list and an optional
// `workspace:` list — so the file stays obvious to edit and there is no hidden
// behaviour:
//
//	plugins:
//	  - greet
//	  - read_file
//
//	workspace:
//	  - ~/.lola/workspace
func Parse(data []byte) (*Config, error) {
	cfg := &Config{}
	section, sub := "", "" // the list block, and inside a guard block its sub-list

	// The provider entry being read, and the indentation of the line that opened
	// it. A field line indented further than its "- " belongs to that entry.
	open, openIndent := -1, 0

	for n, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, " 	\r")
		line = stripInlineComment(line)
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " 	"))

		switch {
		case strings.HasPrefix(trimmed, "plugins:"):
			section, sub, open = "plugins", "", -1

		case strings.HasPrefix(trimmed, "workspace:"):
			section, sub, open = "workspace", "", -1

		case strings.HasPrefix(trimmed, "guard:"):
			section, sub, open = "guard", "", -1

		case strings.HasPrefix(trimmed, "providers:"):
			section, sub, open = "providers", "", -1

		case strings.HasPrefix(trimmed, "protected:"),
			strings.HasPrefix(trimmed, "gated:"),
			strings.HasPrefix(trimmed, "dangerous:"):
			if section != "guard" {
				return nil, fmt.Errorf("config: line %d: %q is only valid inside a guard: block", n+1, trimmed)
			}
			sub = strings.TrimSuffix(trimmed, ":")

		case strings.HasPrefix(trimmed, "- "):
			if section == "" || (section == "guard" && sub == "") {
				return nil, fmt.Errorf("config: line %d: list item outside a list block", n+1)
			}
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			value = strings.Trim(value, `"'"`)
			if value == "" {
				return nil, fmt.Errorf("config: line %d: empty value", n+1)
			}

			if section == "providers" {
				// An entry can be written out in full, one field per line:
				//
				//	providers:
				//	  - name: openrouter
				//	    url: https://openrouter.ai/api/v1
				//	    key: OPENROUTER_API_KEY
				//
				// or as a shorthand, which is handy for a one-off:
				//
				//	providers:
				//	  - local=http://127.0.0.1:11434/v1
				if field, rest, ok := splitField(value); ok {
					cfg.Providers = append(cfg.Providers, ProviderSpec{})
					open, openIndent = len(cfg.Providers)-1, indent
					if err := setProviderField(&cfg.Providers[open], field, rest, n+1); err != nil {
						return nil, err
					}
					continue
				}

				entry, err := parseShorthand(value)
				if err != nil {
					return nil, fmt.Errorf("config: line %d: %w", n+1, err)
				}
				cfg.Providers = append(cfg.Providers, entry)
				open = -1
				continue
			}

			open = -1

			switch {
			case section == "plugins":
				cfg.Plugins = append(cfg.Plugins, value)
			case section == "workspace":
				cfg.Workspace = append(cfg.Workspace, value)
			case sub == "protected":
				cfg.Guard.Protected = append(cfg.Guard.Protected, value)
			case sub == "gated":
				cfg.Guard.Gated = append(cfg.Guard.Gated, value)
			case sub == "dangerous":
				cfg.Guard.Dangerous = append(cfg.Guard.Dangerous, value)
			}

		case open >= 0 && indent > openIndent:
			// A field of the provider entry above it.
			field, rest, ok := splitField(trimmed)
			if !ok {
				return nil, fmt.Errorf("config: line %d: unexpected %q", n+1, trimmed)
			}
			if err := setProviderField(&cfg.Providers[open], field, rest, n+1); err != nil {
				return nil, err
			}

		default:
			return nil, fmt.Errorf("config: line %d: unexpected %q", n+1, trimmed)
		}
	}

	// Check the providers once everything is read, so the error can name one.
	for i, p := range cfg.Providers {
		if p.Name == "" {
			return nil, fmt.Errorf("config: provider at position %d has no name", i+1)
		}
		if p.URL == "" {
			return nil, fmt.Errorf("config: provider %q has no url", p.Name)
		}
	}

	return cfg, nil
}

// splitField reads "field: value". Only a plain field name counts, so a URL is
// never mistaken for one: "name: local" is a field, "local=http://…" is not.
func splitField(line string) (field, value string, ok bool) {
	field, value, found := strings.Cut(line, ":")
	if !found {
		return "", "", false
	}

	field = strings.ToLower(strings.TrimSpace(field))
	value = strings.Trim(strings.TrimSpace(value), `"'"`)

	if field == "" {
		return "", "", false
	}
	for _, r := range field {
		switch {
		case r >= 'a' && r <= 'z', r == '_':
		default:
			return "", "", false
		}
	}
	return field, value, true
}

// setProviderField fills one field of a provider entry being read.
func setProviderField(p *ProviderSpec, field, value string, line int) error {
	switch field {
	case "name":
		p.Name = value
	case "url":
		p.URL = value
	case "key":
		p.Key = value
	default:
		return fmt.Errorf("config: line %d: %q is not a provider field (name, url, key)", line, field)
	}
	return nil
}

// parseShorthand reads "name=url". The key, if the endpoint needs one, is looked
// for in an environment variable named after the provider — NAME_API_KEY — so
// the shorthand never carries a secret either.
func parseShorthand(spec string) (ProviderSpec, error) {
	name, url, found := strings.Cut(spec, "=")
	name, url = strings.TrimSpace(name), strings.TrimSpace(url)

	if !found || name == "" || url == "" {
		return ProviderSpec{}, fmt.Errorf("%q is not name=url, or name: over a url: line", spec)
	}
	return ProviderSpec{Name: name, URL: url, Key: keyEnvFor(name)}, nil
}

// keyEnvFor is the environment variable a provider's key is looked for in: the
// name, upper-cased, with anything that is not a letter or a digit turned into
// an underscore, and _API_KEY on the end.
func keyEnvFor(name string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(name) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String() + "_API_KEY"
}

// stripInlineComment removes a trailing "# comment" from a line, respecting
// quotes so a "#" inside quotes is kept.
func stripInlineComment(line string) string {
	inSingle, inDouble := false, false
	for i, r := range line {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}

// Tree is a set of plugins mounted from a config, in order.
type Tree struct {
	mounted []mountedPlugin
}

type mountedPlugin struct {
	name     string
	plugin   core.Plugin
	teardown func() error
}

// Build mounts every plugin the config lists, in order, using the factories
// registered in the plugin package.
//
// Build is all-or-nothing: if any plugin is unknown or fails to set up, the
// plugins already mounted are unmounted and the error is returned.
func (c *Config) Build(deps plugin.Deps) (*Tree, error) {
	tree := &Tree{}
	for _, name := range c.Plugins {
		p, err := plugin.Build(name, deps)
		if err != nil {
			_ = tree.Unmount()
			return nil, err
		}

		teardown, err := p.Setup()
		if err != nil {
			_ = tree.Unmount()
			return nil, fmt.Errorf("config: setup %s: %w", name, err)
		}

		tree.mounted = append(tree.mounted, mountedPlugin{name: name, plugin: p, teardown: teardown})
	}
	return tree, nil
}

// Unmount unmounts every plugin in reverse mount order, attempting all of them
// even if one teardown fails.
func (t *Tree) Unmount() error {
	var firstErr error
	for i := len(t.mounted) - 1; i >= 0; i-- {
		if td := t.mounted[i].teardown; td != nil {
			if err := td(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	t.mounted = nil
	return firstErr
}

// Names returns the mounted plugin names in mount order.
func (t *Tree) Names() []string {
	names := make([]string, len(t.mounted))
	for i, m := range t.mounted {
		names[i] = m.name
	}
	return names
}

// MountOne adds a single plugin to a running tree, built by name.
//
// It deliberately does not behave like Build. Build is all-or-nothing because
// it runs at startup, where a half-built tree is no use. This runs while the
// harness is up, so an unknown name, or one already mounted, must leave the
// tree exactly as it was rather than tearing down what is working.
func (t *Tree) MountOne(deps plugin.Deps, name string) error {
	for _, m := range t.mounted {
		if m.name == name {
			return fmt.Errorf("config: %s is already mounted", name)
		}
	}

	p, err := plugin.Build(name, deps)
	if err != nil {
		return err
	}

	teardown, err := p.Setup()
	if err != nil {
		return fmt.Errorf("config: setup %s: %w", name, err)
	}

	t.mounted = append(t.mounted, mountedPlugin{name: name, plugin: p, teardown: teardown})
	return nil
}

// UnmountOne removes a single plugin by name and leaves the rest mounted.
func (t *Tree) UnmountOne(name string) error {
	for i, m := range t.mounted {
		if m.name != name {
			continue
		}

		var err error
		if m.teardown != nil {
			err = m.teardown()
		}
		t.mounted = append(t.mounted[:i], t.mounted[i+1:]...)
		return err
	}
	return fmt.Errorf("config: %s is not mounted", name)
}

// Get returns the mounted plugin built under the given name, so a caller can
// reach a capability a plugin contributed. Returns false if it is not mounted.
func (t *Tree) Get(name string) (core.Plugin, bool) {
	for _, m := range t.mounted {
		if m.name == name {
			return m.plugin, true
		}
	}
	return nil, false
}
