package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/adapters"
	"github.com/usually-broken/lola/core/agent"
	"github.com/usually-broken/lola/core/commands"
	"github.com/usually-broken/lola/core/contributions"
	"github.com/usually-broken/lola/core/frontend"
	"github.com/usually-broken/lola/core/guard"
	"github.com/usually-broken/lola/core/home"
	"github.com/usually-broken/lola/core/llm"
	"github.com/usually-broken/lola/core/plugin"
	prompts "github.com/usually-broken/lola/core/prompt"
	"github.com/usually-broken/lola/core/routes"
	"github.com/usually-broken/lola/core/session"
	"github.com/usually-broken/lola/core/settings"
	"github.com/usually-broken/lola/core/tools"
	"github.com/usually-broken/lola/core/workspace"

	// Blank-imported so every plugin's factory registers. Which plugins
	// actually mount is decided by lola.yaml, not by this import.
	_ "github.com/usually-broken/lola/catalog"

	"github.com/usually-broken/lola/core/modes"
	"github.com/usually-broken/lola/core/panes"
	"github.com/usually-broken/lola/core/registry/config"
)

// version is stamped at build time by the release build (see
// .github/workflows/release.yml). Running from a clone leaves it as "dev".
var version = "dev"

// mountedTree is the part of the registry the control needs. Named here so
// main does not have to import the registry package to satisfy it.
type mountedTree interface {
	Names() []string
	Get(name string) (core.Plugin, bool)
	MountOne(plugin.Deps, string) error
	UnmountOne(string) error
}

// consentOf adapts the guard's rules to the seam a plugin sees.
//
// It holds the live rules rather than a copy, so changing the mode here changes
// what happens on the next tool call rather than the next restart.
type consentOf struct{ rules *guard.Rules }

// Mode is the mode in force.
func (c consentOf) Mode() string { return string(c.rules.ModeOf()) }

// Modes lists the modes that can be chosen, in the order they loosen.
func (c consentOf) Modes() []string {
	all := guard.Modes()
	out := make([]string, 0, len(all))
	for _, m := range all {
		out = append(out, string(m))
	}
	return out
}

// SetMode changes it, refusing a mode the guard does not know.
func (c consentOf) SetMode(name string) error {
	m, err := guard.ParseMode(name)
	if err != nil {
		return err
	}
	return c.rules.SetMode(m)
}

// sessionsOf is the conversation store seen through the seam on Deps. The store
// itself is chosen after the tree is built — it may come from a keeper plugin —
// so this reads through a pointer that is filled in once the tree is up.
type sessionsOf struct {
	store *session.Store
}

// List is every saved conversation, newest first.
func (s *sessionsOf) List() ([]plugin.SessionInfo, error) {
	if s.store == nil {
		return nil, fmt.Errorf("no conversation store")
	}

	saved, err := s.store.List()
	if err != nil {
		return nil, err
	}

	out := make([]plugin.SessionInfo, 0, len(saved))
	for _, sess := range saved {
		out = append(out, plugin.SessionInfo{
			ID:       sess.ID,
			Title:    sess.Title,
			Updated:  sess.Updated,
			Pinned:   sess.Pinned,
			Archived: sess.Archived,
			Count:    len(sess.Messages),
		})
	}
	return out, nil
}

// Delete removes one conversation by id.
func (s *sessionsOf) Delete(id string) error {
	if s.store == nil {
		return fmt.Errorf("no conversation store")
	}
	return s.store.Delete(id)
}

// settingsOf is the settings service, seen through the seam on Deps. Like the
// conversation store it is created after the tree is up, because a keeper can
// be what makes settings outlive a run, so this reads through a pointer that is
// filled in once the runtime is assembled.
type settingsOf struct {
	live *settings.Settings
}

// Read is the current settings, as plain data a screen can draw.
func (s *settingsOf) Read() plugin.SettingsView {
	if s.live == nil {
		return plugin.SettingsView{}
	}
	snap := s.live.Snapshot()
	return plugin.SettingsView{
		Model:       snap.Model,
		Mode:        snap.Mode,
		Source:      snap.Source,
		Workspace:   snap.Workspace,
		Protected:   snap.Protected,
		Gated:       snap.Gated,
		Dangerous:   snap.Dangerous,
		ConfigPath:  snap.ConfigPath,
		SessionsDir: snap.SessionsDir,
	}
}

// Write applies one named change. An unknown name is an error rather than a
// shrug: a screen that thinks it saved something should find out it did not.
func (s *settingsOf) Write(what, value string) error {
	if s.live == nil {
		return fmt.Errorf("settings: nothing here keeps settings")
	}

	switch what {
	case "model":
		s.live.SetModel(value)
		return nil
	case "source":
		s.live.SetSource(value)
		return nil
	case "mode":
		return s.live.SetMode(value)
	case "workspace":
		var dirs []string
		if err := json.Unmarshal([]byte(value), &dirs); err != nil {
			return fmt.Errorf("workspace: %w", err)
		}
		return s.live.SetWorkspace(dirs...)
	case "guard":
		var rules struct {
			Protected []string `json:"protected"`
			Gated     []string `json:"gated"`
			Dangerous []string `json:"dangerous"`
		}
		if err := json.Unmarshal([]byte(value), &rules); err != nil {
			return fmt.Errorf("guard: %w", err)
		}
		s.live.SetGuard(rules.Protected, rules.Gated, rules.Dangerous)
		return nil
	}
	return fmt.Errorf("settings: %q is not something this harness can change", what)
}

// treeControl is the plugin tree, seen through the seam on Deps. It is what
// lets an interface list the harness's parts and toggle them while it runs.
type treeControl struct {
	reg  mountedTree
	deps plugin.Deps
}

// Mounted lists what is in the tree now.
func (t *treeControl) Mounted() []string { return t.reg.Names() }

// Get returns a mounted plugin by name, so a caller can ask what a plugin
// actually is rather than only what it is called.
func (t *treeControl) Get(name string) (core.Plugin, bool) { return t.reg.Get(name) }

// Available lists every plugin that exists to be mounted.
func (t *treeControl) Available() []string { return plugin.Names() }

// Mount builds a plugin by name and adds it to the running tree.
func (t *treeControl) Mount(name string) error {
	return t.reg.MountOne(t.deps, name)
}

// Unmount removes a plugin from the running tree.
func (t *treeControl) Unmount(name string) error { return t.reg.UnmountOne(name) }

func main() {
	task := flag.String("task", "", "give the agent a task and run it to completion")
	prompt := flag.String("prompt", "", "ask the model one question and print the reply")
	model := flag.String("model", "gemma4:cloud", "model to use")
	baseURL := flag.String("base-url", "", "OpenAI-compatible endpoint (default: local Ollama)")
	configPath := flag.String("config", config.DefaultPath, "path to the plugin config file")
	frontendName := flag.String("frontend", "", "the frontend plugin to run, by name (see -plugins)")
	frontendAddr := flag.String("addr", "127.0.0.1:8765", "address for a frontend that listens on one")
	var workspaceFlag repeatableFlag
	flag.Var(&workspaceFlag, "workspace", "a directory the agent's file tools may touch (repeatable; defaults to the home workspace)")
	sourceDir := flag.String("source", "", "where the harness's own source lives, so editing it needs approval")
	toolName := flag.String("tool", "", "run one tool by name instead of calling a model")
	toolArgs := flag.String("tool-args", "{}", "JSON arguments for -tool")
	listTools := flag.Bool("tools", false, "list the registered tools and exit")
	listPlugins := flag.Bool("plugins", false, "list every plugin available to mount and exit")
	debug := flag.Bool("debug", false, "print the full conversation after an agent run")

	var thens repeatableFlag
	flag.Var(&thens, "then", "a follow-up task in the same conversation (repeatable)")
	resumeID := flag.String("resume", "", "resume a saved session by id")
	listSessions := flag.Bool("sessions", false, "list saved sessions and exit")
	newSession := flag.Bool("new", false, "start a new session (the default)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("lola " + version)
		return
	}

	fmt.Println("Lola: an agent harness where everything is a plugin.")

	if *listPlugins {
		for _, name := range plugin.Names() {
			fmt.Printf("  %s\n", name)
		}
		return
	}

	// The tree comes from the config file, not from this program. Adding a
	// plugin is a line in lola.yaml.
	ensureConfig(*configPath)

	cfg, err := config.LoadResolved(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	toolRegistry := tools.New()
	commandRegistry := commands.New()

	// The workspace: where the agent's file tools may touch. Flags win, then
	// the config, then the default home workspace.
	roots, err := resolveWorkspace(workspaceFlag, cfg.Workspace)
	if err != nil {
		log.Fatalf("%v", err)
	}

	// The guard: what may run, what needs consent, what is refused. It is
	// installed on the tool registry, so the decision is made in code before
	// any tool call, not asked of the model.
	guardRules := guard.FromSpec(home.Dir(), *sourceDir, guard.Spec{
		Protected: cfg.Guard.Protected,
		Gated:     cfg.Guard.Gated,
		Dangerous: cfg.Guard.Dangerous,
	})
	gate := guard.NewGate(guardRules, guard.RefuseAll{})
	toolRegistry.SetGate(gate)

	// The tree is handed to plugins as a seam, so an interface can show what
	// the harness is made of and toggle it. The control needs the tree and the
	// tree needs the control, so it is filled in straight after the build.
	deps := plugin.Deps{
		Tools:         toolRegistry,
		Workspace:     roots,
		Commands:      commandRegistry,
		Panes:         panes.New(),
		Contributions: contributions.New(),
		Routes:        routes.New(),
		Prompt:        prompts.New(),
	}

	// The conversation store is decided after the tree is up — it may come from
	// a keeper plugin — so the seam is handed over now and filled in later.
	conversations := &sessionsOf{}
	deps.Sessions = conversations
	settingsSeam := &settingsOf{}
	deps.Settings = settingsSeam
	deps.Adapters = adapters.New()
	control := &treeControl{deps: deps}
	deps.Plugins = control

	// The consent seam: how much the guard asks before it acts, offered to
	// plugins so a control for it can come from one rather than from the
	// interface. The rules are live, so a change takes effect on the next call.
	deps.Consent = consentOf{rules: guardRules}

	// The mode seam. It reads which mode is in force off the tree, so the config
	// file decides where the harness starts and nothing here names a mode.
	deps.Modes = modes.New(control)

	tree, err := cfg.Build(deps)
	if err != nil {
		log.Fatalf("failed to build tree: %v", err)
	}
	control.reg = tree
	defer func() {
		if err := tree.Unmount(); err != nil {
			log.Printf("failed to unmount tree: %v", err)
		}
	}()

	fmt.Println("mounted:", strings.Join(tree.Names(), ", "))
	fmt.Println("workspace:", strings.Join(roots.Dirs(), ", "))

	// Where model calls go, decided once for every way of running: the flag if
	// it was given, so an experiment needs no config edit, and otherwise the
	// config file, where the first endpoint listed is the one we start on.
	specs := make([]plugin.ProviderSpec, 0, len(cfg.Providers))
	for _, p := range cfg.Providers {
		specs = append(specs, plugin.ProviderSpec{Name: p.Name, URL: p.URL, Key: p.Key, Kind: p.Kind})
	}
	if *baseURL != "" {
		specs = []plugin.ProviderSpec{{Name: "command line", URL: *baseURL}}
	}
	if len(specs) == 0 {
		specs = []plugin.ProviderSpec{{Name: "local", URL: "http://127.0.0.1:11434/v1"}}
	}

	// Providers added from an interface are kept in the harness home, not in the
	// repo, so nothing written by clicking a button can end up in a commit. The
	// config file wins where both name the same provider.
	savedCfg, err := config.Load(home.ProvidersPath())
	var saved []plugin.ProviderSpec
	if err == nil {
		for _, p := range savedCfg.Providers {
			saved = append(saved, plugin.ProviderSpec{Name: p.Name, URL: p.URL, Key: p.Key, Kind: p.Kind})
		}
	}
	for _, sp := range saved {
		named := false
		for _, existing := range specs {
			if existing.Name == sp.Name {
				named = true
				break
			}
		}
		if !named {
			specs = append(specs, sp)
		}
	}

	// The shapes an endpoint can speak come out of the tree too, so nothing here
	// has to know that one of them is OpenAI-shaped or that there is one at all.
	if deps.Adapters == nil {
		log.Fatalf("no wire formats offered: nothing in the tree can build a provider — add a model adapter to %s", *configPath)
	}

	// And where calls go comes out of the tree too: which endpoints exist, and
	// what shape any of them speaks, is a plugin's business.
	modelCalls, ok := plugin.Find[plugin.Router](control)
	if !ok {
		log.Fatalf("no llmrouter mounted: nothing in the tree can send a model call — add one to %s", *configPath)
	}
	if err := modelCalls.Seed(specs, saved, home.ProvidersPath(), deps.Adapters); err != nil {
		log.Fatalf("%v", err)
	}

	// The store sessions are kept in comes out of the tree, so the loop, the
	// frontends and this command line all read the same files through the same
	// one. A config with no sessions plugin keeps them here instead, because a
	// harness that quietly stops saving is worse than one that keeps them.
	sessionStore := session.NewStore(sessionsDir())
	if keeper, ok := plugin.Find[session.Keeper](control); ok {
		sessionStore = keeper.Sessions()
	}
	conversations.store = sessionStore

	// A frontend, when one is asked for by name. Which frontends exist is the
	// tree's business rather than this program's: anything mounted that is one
	// can be run, under the name the person typed.
	if *frontendName != "" {
		// A frontend that listens needs to know where, and it needs to hear
		// that before its Frontend() is taken.
		if p, ok := control.Get(*frontendName); ok {
			if w, ok := p.(interface{ SetAddr(string) }); ok {
				w.SetAddr(*frontendAddr)
			}
		}
		rt, err := buildRuntime(control, toolRegistry, roots, guardRules, *model, *configPath, modelCalls, sessionStore, settingsSeam)
		if err != nil {
			log.Fatalf("%v", err)
		}
		fe, err := frontendFromTree(control, *frontendName)
		if err != nil {
			log.Fatalf("%v", err)
		}
		// A frontend that can ask a person takes over approval; otherwise the
		// refuse-all default stays.
		if ap, ok := fe.(interface{ Approver() guard.Approver }); ok {
			gate.SetApprover(ap.Approver())
		}
		if err := fe.Run(context.Background(), rt); err != nil {
			log.Fatalf("frontend: %v", err)
		}
		return
	}

	if *listTools {
		for _, t := range toolRegistry.Names() {
			tool, _ := toolRegistry.Get(t)
			fmt.Printf("  %-12s %s\n", t, tool.Description())
		}
		return
	}

	if *toolName != "" {
		out, err := toolRegistry.Run(context.Background(), *toolName, json.RawMessage(*toolArgs))
		if err != nil {
			log.Fatalf("tool call failed: %v", err)
		}
		fmt.Println(out)
		return
	}

	if *prompt == "" && *task == "" && !*listSessions {
		return
	}

	// Where model calls go, decided once above rather than here.
	provider := modelCalls

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if *prompt != "" {
		resp, err := provider.Complete(ctx, llm.Request{
			Model:    *model,
			Messages: []llm.Message{{Role: llm.RoleUser, Content: *prompt}},
		})
		if err != nil {
			log.Fatalf("model call failed: %v", err)
		}
		fmt.Println(resp.Content)
		return
	}

	// The agent loop is mounted as a plugin too, so the loop driving this run
	// is the one the config chose.
	loop, err := agentFromTree(control)
	if err != nil {
		log.Fatalf("%v", err)
	}

	// Session handling: list, resume, or start fresh.
	if *listSessions {
		list, err := sessionStore.List()
		if err != nil {
			log.Fatalf("%v", err)
		}
		if len(list) == 0 {
			fmt.Println("no saved sessions")
			return
		}
		for _, s := range list {
			flags := ""
			if s.Pinned {
				flags += " [pinned]"
			}
			if s.Archived {
				flags += " [archived]"
			}
			fmt.Printf("  %s  %-40s %d messages%s\n", s.ID, s.Title, len(s.Messages), flags)
		}
		return
	}

	if *resumeID != "" {
		if err := loop.Resume(*resumeID); err != nil {
			log.Fatalf("resume %s: %v", *resumeID, err)
		}
		fmt.Printf("resumed session %s\n", *resumeID)
	}

	if *newSession {
		loop.Reset()
	}

	// Watch it work, if this loop can report its steps. Nothing here knows which
	// plugin drives the loop; it only knows what the loop can do.
	if w, ok := loop.(agent.Watcher); ok {
		w.Watch(func(step int, msg llm.Message) {
			if len(msg.ToolCalls) == 0 {
				fmt.Printf("  step %d: model answered\n", step)
				return
			}
			for _, call := range msg.ToolCalls {
				fmt.Printf("  step %d: calls %s %s\n", step, call.Name, string(call.Arguments))
			}
		})
	}

	// Print the first task's answer, then each follow-up's.
	printAnswer := func(res agent.Result) {
		fmt.Printf("finished in %d step(s)\n", res.Steps)
		fmt.Println(res.Content)
	}

	res, err := loop.Run(ctx, provider, *model, *task)
	if err != nil {
		log.Fatalf("agent run failed: %v", err)
	}
	printAnswer(res)

	// Each -then is a follow-up in the same conversation, so the model sees
	// what was said before. This is how you check the agent remembers.
	for _, follow := range thens {
		fmt.Printf("\n› %s\n", follow)
		res, err = loop.Run(ctx, provider, *model, follow)
		if err != nil {
			log.Fatalf("agent run failed: %v", err)
		}
		printAnswer(res)
	}

	if *debug {
		for i, m := range res.Messages {
			b, _ := json.Marshal(m)
			fmt.Printf("  [%d] %s\n", i, string(b))
		}
	}
}

// repeatableFlag collects every occurrence of a flag into a slice.
type repeatableFlag []string

func (r *repeatableFlag) String() string { return strings.Join(*r, ", ") }
func (r *repeatableFlag) Set(v string) error {
	*r = append(*r, v)
	return nil
}

// resolveWorkspace picks the roots the agent's file tools may touch: the flags
// if any were given, else the config, else the default home workspace.
func resolveWorkspace(flags []string, fromConfig []string) (*workspace.Roots, error) {
	switch {
	case len(flags) > 0:
		return workspace.New(flags...)
	case len(fromConfig) > 0:
		return workspace.New(fromConfig...)
	default:
		dir := home.WorkspaceDir()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("workspace: create default workspace: %w", err)
		}
		return workspace.New(dir)
	}
}

// sessionsDir is where sessions live: the home directory's sessions folder, so
// saving and listing always agree.
func sessionsDir() string {
	return home.SessionsDir()
}

// frontendFromTree pulls a frontend out of the mounted tree by the name it was
// asked for, which is the one place a plugin's name legitimately comes from a
// person rather than from this program.
func frontendFromTree(plugins plugin.Plugins, name string) (frontend.Frontend, error) {
	inst, ok := plugins.Get(name)
	if !ok {
		return nil, fmt.Errorf("no %s mounted: add it to the config, or ask for one that is there (%v)", name, plugins.Mounted())
	}
	f, ok := inst.(interface{ Frontend() frontend.Frontend })
	if !ok {
		return nil, fmt.Errorf("the mounted %s is not a frontend", name)
	}
	return f.Frontend(), nil
}

//go:embed lola.yaml
var defaultConfig []byte

// ensureConfig writes a starting config when there isn't one. Installs that do not run
// `make install` — from a package manager, or a downloaded binary — otherwise arrive with
// a program and nothing for it to mount.
//
// It never writes over an existing file: an upgrade that replaced a config somebody had
// been editing would be the worst thing this could do.
func ensureConfig(path string) {
	// Write where loading would look. The default path is relative to the current
	// directory, with a fallback to the home directory, so writing to the argument alone
	// puts the file somewhere nothing will ever read it from.
	if path == config.DefaultPath {
		path = home.ConfigPath()
	}
	if _, err := os.Stat(path); err == nil {
		return
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Printf("no config at %s and could not create its directory: %v", path, err)
			return
		}
	}
	if err := os.WriteFile(path, defaultConfig, 0o644); err != nil {
		log.Printf("no config at %s and could not write one: %v", path, err)
		return
	}
	log.Printf("wrote a starting config to %s", path)
}

// buildRuntime assembles what a frontend needs from the mounted tree.
func buildRuntime(plugins plugin.Plugins, toolRegistry *tools.Registry, roots *workspace.Roots, guardRules *guard.Rules, model, configPath string, provider llm.Provider, sessionStore *session.Store, settingsSeam *settingsOf) (frontend.Runtime, error) {
	loop, err := agentFromTree(plugins)
	if err != nil {
		return frontend.Runtime{}, err
	}

	live := keepSettings(settings.New(
		model, roots, guardRules, plugins.Mounted(), plugin.Names(),
		configPath, sessionsDir(),
	), plugins)
	if settingsSeam != nil {
		settingsSeam.live = live
	}

	return frontend.Runtime{
		Agent:       loop,
		Provider:    provider,
		Tools:       toolRegistry,
		Plugins:     plugins.Mounted(),
		Model:       model,
		ConfigPath:  configPath,
		Sessions:    sessionStore,
		SessionsDir: sessionsDir(),
		Workspace:   roots,
		Guard:       guardRules,
		Settings:    live,
	}, nil
}

// agentFromTree asks the tree for whatever can drive a task, rather than for a
// plugin by name. Which plugin that turns out to be is the config file's
// business, not this program's.
func agentFromTree(plugins plugin.Plugins) (agent.Agent, error) {
	a, ok := plugin.Find[agent.Agent](plugins)
	if !ok {
		return nil, fmt.Errorf("no agent loop mounted: nothing in the tree can drive a task — add one to %s", config.DefaultPath)
	}
	return a, nil
}
