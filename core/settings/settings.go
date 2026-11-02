// Package settings is the settings seam: one place that knows what the running
// harness can be told, and how to tell it.
//
// Every frontend shows the same settings through this, so the terminal, the
// browser, and anything built later do not each grow their own copy of "what
// can be changed". It holds the live objects rather than copies, so a change
// here is a change the running agent sees on its next call.
package settings

import (
	"sync"

	"github.com/usually-broken/lola/core/guard"
	"github.com/usually-broken/lola/core/workspace"
)

// Snapshot is everything a settings screen needs in order to draw itself.
type Snapshot struct {
	Model       string   `json:"model"`
	Mode        string   `json:"mode"`
	Workspace   []string `json:"workspace"`
	Source      string   `json:"source"`
	Protected   []string `json:"protected"`
	Gated       []string `json:"gated"`
	Dangerous   []string `json:"dangerous"`
	Mounted     []string `json:"mounted"`
	Available   []string `json:"available"`
	ConfigPath  string   `json:"configPath"`
	SessionsDir string   `json:"sessionsDir"`
}

// Settings is what a frontend can read and change while the harness runs.
type Settings struct {
	mu      sync.RWMutex
	model   string
	roots   *workspace.Roots
	rules   *guard.Rules
	mounted []string
	avail   []string
	config  string
	session string

	// keeper is where choices are kept between runs, once Use has been handed
	// one. Nil means this view keeps them in memory only, which is what a test
	// wants and what a harness with no keep plugin mounted gets.
	keeper Keeper
}

// New builds the settings view over the live objects.
func New(model string, roots *workspace.Roots, rules *guard.Rules, mounted, available []string, configPath, sessionsDir string) *Settings {
	return &Settings{
		model:   model,
		roots:   roots,
		rules:   rules,
		mounted: append([]string(nil), mounted...),
		avail:   append([]string(nil), available...),
		config:  configPath,
		session: sessionsDir,
	}
}

// Model is the model tasks run against.
func (s *Settings) Model() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model
}

// SetModel changes the model. It takes effect on the next run, and is kept if
// this view has somewhere to keep it.
func (s *Settings) SetModel(model string) {
	s.mu.Lock()
	s.model = model
	s.mu.Unlock()
	s.keep("model", model)
}

// Snapshot reports everything at once.
func (s *Settings) Snapshot() Snapshot {
	s.mu.RLock()
	out := Snapshot{
		Model:       s.model,
		Mounted:     append([]string(nil), s.mounted...),
		Available:   append([]string(nil), s.avail...),
		ConfigPath:  s.config,
		SessionsDir: s.session,
	}
	s.mu.RUnlock()

	if s.roots != nil {
		out.Workspace = s.roots.Dirs()
	}
	if s.rules != nil {
		out.Source = s.rules.SourceOf()
		out.Mode = string(s.rules.ModeOf())
		out.Protected, out.Gated, out.Dangerous = s.rules.Lists()
	}
	return out
}

// SetWorkspace replaces the roots the file tools may touch.
func (s *Settings) SetWorkspace(dirs ...string) error {
	if s.roots == nil {
		return nil
	}
	return s.roots.Set(dirs...)
}

// SetSource changes where the harness's own source lives. An empty string
// disarms the rule.
func (s *Settings) SetSource(dir string) {
	if s.rules != nil {
		s.rules.SetSource(dir)
	}
}

// SetGuard replaces the guard's three lists. A nil list leaves that one alone.
func (s *Settings) SetGuard(protected, gated, dangerous []string) {
	if s.rules == nil {
		return
	}
	p, g, d := s.rules.Lists()
	if protected != nil {
		p = protected
	}
	if gated != nil {
		g = gated
	}
	if dangerous != nil {
		d = dangerous
	}
	s.rules.SetLists(p, g, d)
}

// ScopeGlobal is where a choice that applies to new sessions lives. It is the
// same string the keeper uses: a scope is a scope, whoever is reading it.
const ScopeGlobal = "global"

// Keeper is where a choice is kept between runs.
//
// Declared here rather than imported from the plugin package so this view
// depends on the shape of the thing and not on the tree that happens to provide
// it. Anything with these two methods will do, which is how the keep plugin
// plugs in without the core naming it.
type Keeper interface {
	// Get answers with what was kept, if anything was.
	Get(scope, key string) (string, bool)

	// Set keeps a value.
	Set(scope, key, value string) error
}

// Use hands this view somewhere to keep its choices, and applies the ones
// already kept.
//
// This is what replaced writing the file here. The core no longer knows where
// anything is stored — it asks for a capability, and a plugin provides it. A
// view with no keeper still works; it simply forgets, which is honest.
func (s *Settings) Use(keeper Keeper) {
	s.mu.Lock()
	s.keeper = keeper
	s.mu.Unlock()

	if keeper == nil {
		return
	}

	if value, ok := keeper.Get(ScopeGlobal, "model"); ok && value != "" {
		s.mu.Lock()
		s.model = value
		s.mu.Unlock()
	}

	// Parsed rather than trusted: an unknown mode in a file is a typo, and a
	// typo must never be the thing that loosens the guard.
	if value, ok := keeper.Get(ScopeGlobal, "mode"); ok && value != "" && s.rules != nil {
		if m, err := guard.ParseMode(value); err == nil {
			_ = s.rules.SetMode(m)
		}
	}
}

// keep records a choice, if there is anywhere to record it. Failing to keep it
// is not a reason to fail the change: the change is already in force.
func (s *Settings) keep(key, value string) {
	s.mu.RLock()
	keeper := s.keeper
	s.mu.RUnlock()

	if keeper == nil {
		return
	}
	_ = keeper.Set(ScopeGlobal, key, value)
}

// SetMode changes how much consent the guard asks for. An unknown mode is
// refused rather than guessed at, and the choice is kept if this view has
// somewhere to keep it.
func (s *Settings) SetMode(mode string) error {
	if s.rules == nil {
		return nil
	}
	m, err := guard.ParseMode(mode)
	if err != nil {
		return err
	}
	if err := s.rules.SetMode(m); err != nil {
		return err
	}
	s.keep("mode", string(m))
	return nil
}
