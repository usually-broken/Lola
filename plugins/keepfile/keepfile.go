// Package keepfile remembers choices between runs, in one file in the harness
// home.
//
// It is a plugin rather than part of the core for the usual reason: the core
// should not have to know what a choice means. Anything in the tree can keep a
// value here — a model, a mode, or something a plugin made up — and they all
// land in the same file, so there is one place to look and one place to back up.
//
// Values are scoped. "global" is the default a new session starts from;
// "session:<id>" belongs to one conversation. Nothing here interprets either.
package keepfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/home"
	"github.com/usually-broken/lola/core/plugin"
)

// ScopeGlobal is where a choice that applies to new sessions lives.
const ScopeGlobal = "global"

// ScopeSession is the prefix for a choice that belongs to one conversation.
const ScopeSession = "session:"

func init() {
	plugin.Register("keep", func(plugin.Deps) (core.Plugin, error) {
		return &Plugin{path: home.SettingsPath()}, nil
	})
}

// Plugin keeps choices in a JSON file: one map of values per scope.
type Plugin struct {
	mu   sync.Mutex
	path string
	data map[string]map[string]string
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "keep" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind:  plugin.KindKeeper,
		What:  "Remembers settings between runs, so the harness starts how you left it.",
		Vital: true,
	}
}

// Setup reads the file. A missing or unreadable file is not a failure: the
// harness starts on its defaults, and the next choice overwrites the file.
func (p *Plugin) Setup() (func() error, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data = p.load()
	return func() error { return nil }, nil
}

// Get answers with what was kept, if anything was.
func (p *Plugin) Get(scope, key string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	values, ok := p.data[scope]
	if !ok {
		return "", false
	}
	value, ok := values[key]
	return value, ok
}

// Set keeps a value and writes the file. A write that fails is reported rather
// than swallowed: a choice that silently vanishes is worse than an error, and
// the caller can still carry on with the value in memory.
func (p *Plugin) Set(scope, key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.data == nil {
		p.data = map[string]map[string]string{}
	}
	if p.data[scope] == nil {
		p.data[scope] = map[string]string{}
	}
	p.data[scope][key] = value
	return p.write()
}

// load reads the file, and moves a flat file — the shape before scopes existed —
// into the global scope, so nobody loses the settings they already had.
func (p *Plugin) load() map[string]map[string]string {
	data, err := os.ReadFile(p.path)
	if err != nil {
		return map[string]map[string]string{}
	}

	var scoped map[string]map[string]string
	if err := json.Unmarshal(data, &scoped); err == nil {
		return scoped
	}

	// The old shape: a single map of key to value, with no scope. Unmarshalling
	// it as scoped fails, which is how the two shapes are told apart.
	var flat map[string]string
	if err := json.Unmarshal(data, &flat); err == nil {
		return map[string]map[string]string{ScopeGlobal: flat}
	}

	return map[string]map[string]string{}
}

// write saves the file: a temporary file first, then rename, so a crash halfway
// through cannot leave half a settings file behind.
func (p *Plugin) write() error {
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(p.data, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(p.path), "settings-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, p.path)
}

// Assert at compile time that this plugin is a keeper.
var _ plugin.Keeper = (*Plugin)(nil)
