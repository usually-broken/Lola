// Package web is the browser frontend: the harness served as a web page.
//
// It is a frontend plugin like the TUI, implementing the same seam. It knows
// nothing about how the agent loop, the tools, the sessions or the guard work
// — it is handed a Runtime and speaks HTTP instead of drawing a terminal.
//
// The page talks to it over Server-Sent Events, so a run's steps arrive as
// they happen rather than all at the end.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/agent"
	"github.com/usually-broken/lola/core/commands"
	"github.com/usually-broken/lola/core/frontend"
	"github.com/usually-broken/lola/core/guard"
	"github.com/usually-broken/lola/core/llm"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/prompt"
	"github.com/usually-broken/lola/core/routes"
	"github.com/usually-broken/lola/core/session"
	"github.com/usually-broken/lola/core/tools"
)

//go:embed index.html
var assets embed.FS

// DefaultAddr is where the interface listens when nothing else is set. It is
// loopback only: the interface is for this machine, not the network.
const DefaultAddr = "127.0.0.1:8765"

func init() {
	plugin.Register("web", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{addr: DefaultAddr, commands: d.Commands, tree: d.Plugins, panes: d.Panes, modes: d.Modes, contributions: d.Contributions, routes: d.Routes, ui: &ui{prompt: d.Prompt, tools: d.Tools}}, nil
	})
}

// Plugin serves the harness as a web page.
type Plugin struct {
	addr          string
	commands      *commands.Registry
	tree          plugin.Plugins
	panes         plugin.Panes
	modes         plugin.Modes
	contributions plugin.Contributions
	routes        *routes.Registry
	ui            *ui
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "web" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindUI,
		What: "The page: transcript, composer, and the socket everything else draws into.",
	}
}

// Setup has nothing to do until the interface is actually run.
func (p *Plugin) Setup() (func() error, error) { return func() error { return nil }, nil }

// Frontend returns the seam this plugin contributes: a browser frontend.
func (p *Plugin) Frontend() frontend.Frontend {
	u := p.theUI()
	u.addr = p.addr
	return u
}

// Approver returns the consent seam. It lives on the frontend rather than on
// the plugin wrapper, because the frontend is the object the harness actually
// holds and checks. Hung off the plugin it was never reached, so the guard kept
// its refuse-all default and every gated call failed instead of asking.
func (u *ui) Approver() guard.Approver { return u }

// Say is the notify seam from the other side: something in the tree has said a
// line, and it arrives on the page.
//
// It lives on the plugin wrapper because the wrapper is the object the tree
// holds, and the page it reaches is the same one the frontend is. Saying
// something before the server has started is not an error — the line has
// nowhere to go and is dropped, which is the honest reading of nobody watching.
func (p *Plugin) Say(text string) {
	p.theUI().broadcast(event{Kind: "note", Text: text})
}

// Assert at compile time that a plugin asking the tree for something to say
// things to finds this frontend, and that a plugin asking to run a task finds
// it too.
var (
	_ plugin.Notify = (*Plugin)(nil)
	_ plugin.Run    = (*Plugin)(nil)
)

// SetAddr changes where the interface will listen. It must be called before
// the frontend is run.
func (p *Plugin) SetAddr(addr string) { p.addr = addr }

// theUI returns the one interface instance, creating it if needed, so that the
// frontend and the approver are the same object.
func (p *Plugin) theUI() *ui {
	if p.ui == nil {
		p.ui = &ui{}
	}
	if p.ui.commands == nil {
		p.ui.commands = p.commands
	}
	if p.ui.tree == nil {
		p.ui.tree = p.tree
	}
	if p.ui.panes == nil {
		p.ui.panes = p.panes
	}
	if p.ui.modes == nil {
		p.ui.modes = p.modes
	}
	if p.ui.contributions == nil {
		p.ui.contributions = p.contributions
	}
	if p.ui.routes == nil {
		p.ui.routes = p.routes
	}
	return p.ui
}

// ui is the browser frontend itself.
type ui struct {
	addr          string
	rt            frontend.Runtime
	store         *session.Store
	commands      *commands.Registry
	tree          plugin.Plugins
	panes         plugin.Panes
	modes         plugin.Modes
	contributions plugin.Contributions
	routes        *routes.Registry

	// prompt is what the model is told, shown part by part at /api/prompt.
	prompt *prompt.Registry

	// tools are the functions the model may call, shown with the prompt so the
	// whole request can be read in one place.
	tools *tools.Registry

	mu      sync.Mutex
	clients map[chan event]struct{}

	// unplaced remembers what has already been complained about, so a missing
	// place is said once rather than on every poll.
	unplaced map[string]bool
	pending  map[string]chan guard.Approval
	nextID   int
	busy     bool
	current  string
}

// event is one thing sent to the browser over the stream.
type event struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
	Tool string `json:"tool,omitempty"`
	Busy bool   `json:"busy,omitempty"`

	// ID identifies an approval question, so the answer can be matched to it.
	ID string `json:"id,omitempty"`
}

// answeredBy puts the paths plugins offered in front of this interface's own.
//
// A plugin's path is looked up per request rather than mounted once, so a
// plugin can offer one at any point — including after this server has started —
// and never has to be built before the interface is. Nothing offered means the
// interface answers everything itself, exactly as it did before this existed.
func (u *ui) answeredBy(next http.Handler) http.Handler {
	if u.routes == nil {
		return next
	}
	return u.routes.Serve(next)
}

// Run serves the interface until the context is cancelled.
func (u *ui) Run(ctx context.Context, rt frontend.Runtime) error {
	u.rt = rt
	u.store = rt.Sessions
	u.clients = make(map[chan event]struct{})
	u.pending = make(map[string]chan guard.Approval)
	u.registerCommands()

	mux := http.NewServeMux()
	mux.HandleFunc("/", u.index)
	mux.HandleFunc("/api/state", u.state)
	mux.HandleFunc("/api/prompt", u.promptGet)
	mux.HandleFunc("/api/prompt/full", u.promptFull)
	mux.HandleFunc("/api/events", u.events)
	mux.HandleFunc("/api/send", u.send)
	mux.HandleFunc("/api/command", u.command)
	mux.HandleFunc("/api/panes", u.panesGet)
	mux.HandleFunc("/api/contributions", u.contributionsGet)
	mux.HandleFunc("/api/contribution/set", u.contributionSet)
	mux.HandleFunc("/api/providers", u.providersGet)
	mux.HandleFunc("/api/providers/set", u.providersSet)
	mux.HandleFunc("/api/providers/add", u.providersAdd)
	mux.HandleFunc("/api/providers/remove", u.providersRemove)
	mux.HandleFunc("/api/approve", u.approve)
	mux.HandleFunc("/api/history", u.history)
	mux.HandleFunc("/api/commands", u.commandsList)

	srv := &http.Server{
		Addr:              u.addr,
		Handler:           u.answeredBy(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	log.Printf("web interface on http://%s", u.addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("web: %w", err)
	}
	return nil
}

// --- pages and data -------------------------------------------------------

func (u *ui) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	page, err := assets.ReadFile("index.html")
	if err != nil {
		http.Error(w, "web: page missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

// stateResponse is what the status bar needs.
type stateResponse struct {
	Model       string   `json:"model"`
	Plugins     []string `json:"plugins"`
	ConfigPath  string   `json:"configPath"`
	SessionsDir string   `json:"sessionsDir"`
	Workspace   []string `json:"workspace"`
	Session     string   `json:"session"`
	Busy        bool     `json:"busy"`
}

func (u *ui) state(w http.ResponseWriter, r *http.Request) {
	s := stateResponse{
		Model:       u.model(),
		Plugins:     u.rt.Plugins,
		ConfigPath:  u.rt.ConfigPath,
		SessionsDir: u.rt.SessionsDir,
		Session:     u.current,
	}
	if u.rt.Workspace != nil {
		s.Workspace = u.rt.Workspace.Dirs()
	}

	u.mu.Lock()
	s.Busy = u.busy
	u.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s)
}

// sessionsResponse is one row in the session list.
// --- the live stream ------------------------------------------------------

// events holds the connection open and pushes events down it. That is what
// makes a run's steps appear as they happen.
func (u *ui) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "web: streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan event, 64)
	u.addClient(ch)
	defer u.removeClient(ch)

	u.push(ch, event{Kind: "ready"})

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

// --- sending a message ----------------------------------------------------

type sendRequest struct {
	Text string `json:"text"`
}

func (u *ui) send(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "web: POST only", http.StatusMethodNotAllowed)
		return
	}

	var req sendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "web: bad request", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		http.Error(w, "web: empty message", http.StatusBadRequest)
		return
	}

	// One run at a time: the agent holds a conversation, so two at once would
	// interleave into nonsense.
	u.mu.Lock()
	if u.busy {
		u.mu.Unlock()
		http.Error(w, "web: already running", http.StatusConflict)
		return
	}
	u.busy = true
	u.mu.Unlock()

	u.broadcast(event{Kind: "user", Text: text})
	u.broadcast(event{Kind: "busy", Busy: true})

	go u.run(text)

	w.WriteHeader(http.StatusAccepted)
}

// Do is the run seam: a task in, each step as it happens, the answer out.
//
// It lives on the plugin wrapper because the wrapper is what the tree holds, so
// anything in the tree that wants the harness to do something asks for this
// capability instead of assembling a provider, a model and a loop by hand.
func (p *Plugin) Do(ctx context.Context, task string, onStep func(tool, args string)) (string, error) {
	u := p.theUI()

	// The run seam takes the same route the page's own send takes, narrating
	// each stage to whoever is watching, so a task started by a plugin looks
	// exactly like a task started by a person at the keyboard.
	u.mu.Lock()
	if u.busy {
		u.mu.Unlock()
		return "", fmt.Errorf("web: already running")
	}
	u.busy = true
	u.mu.Unlock()

	u.broadcast(event{Kind: "user", Text: task})
	u.broadcast(event{Kind: "busy", Busy: true})
	defer func() {
		u.mu.Lock()
		u.busy = false
		u.mu.Unlock()
		u.broadcast(event{Kind: "busy", Busy: false})
	}()

	answer, err := u.do(ctx, task, onStep)
	if err != nil {
		u.broadcast(event{Kind: "error", Text: err.Error()})
		return "", err
	}

	u.broadcast(event{Kind: "answer", Text: answer})
	u.broadcast(event{Kind: "sessions"})
	return answer, nil
}

// do drives one task and reports it as it goes. The page's own send handler
// comes through here too, so a task is started in one place rather than once
// per caller.
func (u *ui) do(ctx context.Context, task string, onStep func(tool, args string)) (string, error) {
	// Text as it is written, so a long answer starts arriving instead of
	// appearing all at once.
	if watcher, ok := u.rt.Agent.(interface{ WatchDelta(func(string)) }); ok {
		watcher.WatchDelta(func(delta string) {
			u.broadcast(event{Kind: "partial", Text: delta})
		})
	}

	// Speech, as it happens: what it says before a tool is a message, and the
	// page draws it as one rather than as a label on the call.
	if watcher, ok := u.rt.Agent.(interface{ WatchSay(func(string)) }); ok {
		watcher.WatchSay(func(text string) {
			u.broadcast(event{Kind: "say", Text: text})
		})
	}

	watch := func(call llm.ToolCall) {
		// What is shown is the reason the model gave, not the argument blob: the
		// blob is for the tool, the sentence is for the person.
		detail := tools.ReasonOrArgs(call.Arguments)
		u.broadcast(event{Kind: "step", Tool: call.Name, Text: detail})
		if onStep != nil {
			onStep(call.Name, string(call.Arguments))
		}
	}

	var res agent.Result
	var err error

	// The loop can report each tool call as it happens, which is what makes
	// the transcript live rather than a single lump at the end.
	if w, ok := u.rt.Agent.(interface {
		RunWatched(context.Context, llm.Provider, string, string, func(llm.ToolCall)) (agent.Result, error)
	}); ok {
		res, err = w.RunWatched(ctx, u.rt.Provider, u.model(), task, watch)
	} else {
		res, err = u.rt.Agent.Run(ctx, u.rt.Provider, u.model(), task)
	}

	if err != nil {
		return "", err
	}

	// Remember which session this run belongs to, so /pin and /archive act on
	// the right one without being told.
	if s, ok := u.rt.Agent.(interface{ SessionID() string }); ok {
		u.mu.Lock()
		u.current = s.SessionID()
		u.mu.Unlock()
	}

	return res.Content, nil
}

// run drives one task for the page and reports it on the stream.
func (u *ui) run(task string) {
	defer func() {
		u.mu.Lock()
		u.busy = false
		u.mu.Unlock()
		u.broadcast(event{Kind: "busy", Busy: false})
	}()

	answer, err := u.do(context.Background(), task, nil)
	if err != nil {
		u.broadcast(event{Kind: "error", Text: err.Error()})
		return
	}

	u.broadcast(event{Kind: "answer", Text: answer})
	u.broadcast(event{Kind: "sessions"})
}

// historyResponse is one message of the conversation, for the transcript.
type historyMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
	Tool string `json:"tool,omitempty"`
}

// history returns the conversation so far, so the page can redraw it after a
// resume rather than showing an empty transcript.
func (u *ui) history(w http.ResponseWriter, r *http.Request) {
	msgs := u.rt.Agent.History()

	out := make([]historyMessage, 0, len(msgs))
	for _, m := range msgs {
		row := historyMessage{Role: string(m.Role), Text: m.Content}
		if len(m.ToolCalls) > 0 {
			row.Tool = m.ToolCalls[0].Name
		}
		out = append(out, row)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// commandsResponse is one row in the command list.
type commandInfo struct {
	Name        string `json:"name"`
	Usage       string `json:"usage,omitempty"`
	Description string `json:"description"`
}

// commandsList reports what the interface can do, so the page can show it
// rather than expecting a person to know /help exists.
func (u *ui) commandsList(w http.ResponseWriter, r *http.Request) {
	all := u.commands.All()

	out := make([]commandInfo, 0, len(all))
	for _, c := range all {
		out = append(out, commandInfo{Name: c.Name, Usage: c.Usage, Description: c.Description})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// model is the one the next run will use. Settings are the source of truth
// when they are present, so a change made in a settings screen takes effect.
func (u *ui) model() string {
	if u.rt.Settings != nil {
		return u.rt.Settings.Model()
	}
	return u.rt.Model
}

// settingsGet reports everything a settings screen needs to draw itself.

// panesGet lists the panels plugins have offered, so the page can put them in
// the places the frame provides. An empty list is normal: it means nothing has
// offered one.
func (u *ui) panesGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if u.panes == nil {
		_, _ = w.Write([]byte("[]"))
		return
	}
	_ = json.NewEncoder(w).Encode(u.panes.All())
}

// contributionsGet lists what plugins have offered the page: chips, toggles,
// selectors, panels. Each kind is drawn by the page in its own way, and a place
// the page does not know is simply not drawn.
func (u *ui) contributionsGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if u.contributions == nil {
		_, _ = w.Write([]byte("[]"))
		return
	}

	// Only what this interface has somewhere to put. Anything else is dropped
	// here rather than handed to the page, which would have nowhere to draw it
	// and no way to say so. The page cannot report anything, so this is where
	// the saying happens.
	// A plugin that has been unmounted offered its contributions once, and the
	// registry keeps them. Left alone, a switch flipped off leaves its chip
	// behind, and flipped on again draws a second — so only what a mounted
	// plugin offered is sent, and only once per name.
	mounted := map[string]bool{}
	if u.tree != nil {
		for _, name := range u.tree.Mounted() {
			mounted[name] = true
		}
	}

	seen := map[string]bool{}
	kept := make([]plugin.Contribution, 0)
	for _, c := range u.contributions.All() {
		if !knowsPlace(c.Place) {
			u.reportUnplaced(c)
			continue
		}
		if u.tree != nil && !mounted[c.Owner] {
			continue
		}
		key := c.Owner + "/" + c.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		kept = append(kept, c)
	}

	_ = json.NewEncoder(w).Encode(kept)
}

// contributionSet applies a change to a contribution whose value can change.
//
// The name is the contribution's own, and the first match wins: two plugins
// offering the same name is a collision, and the one that loses is better off
// being renamed than silently winning by mounting order.
func (u *ui) contributionSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "web: POST only", http.StatusMethodNotAllowed)
		return
	}
	if u.contributions == nil {
		http.Error(w, "web: nothing is collecting contributions", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "web: bad request", http.StatusBadRequest)
		return
	}

	for _, c := range u.contributions.All() {
		if c.Name != req.Name {
			continue
		}
		if c.Set == nil {
			http.Error(w, "web: "+req.Name+" cannot be changed", http.StatusBadRequest)
			return
		}
		if err := c.Set(req.Value); err != nil {
			http.Error(w, "web: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// The plugin has changed something, so whoever else is watching is
		// told. What it changed is the plugin's business; this is only "ask
		// again".
		u.broadcast(event{Kind: "contributions"})
		u.broadcast(event{Kind: "state"})
		w.WriteHeader(http.StatusAccepted)
		return
	}

	http.Error(w, "web: no contribution called "+req.Name, http.StatusBadRequest)
}

// places are the spots this interface will draw a plugin's offering into. A
// contribution that names anything else is refused rather than guessed at, so a
// typo shows up in the log instead of vanishing.
//
// "composer" is the box itself: the place the page keeps for whoever draws the
// field you type into.
// The places this interface can fill. A contribution naming one that is not here is
// dropped with a complaint in the log, which is how a ball that never appeared was
// tracked down to the page refusing the place it asked for.
var places = []string{"above-composer", "composer-tools", "composer", "voice-mode"}

// knowsPlace says whether this interface has somewhere to put a contribution.
func knowsPlace(place string) bool {
	for _, p := range places {
		if p == place {
			return true
		}
	}
	return false
}

// reportUnplaced says so, once per thing. A control that never appears is far
// harder to track down than a line naming what asked for what, and complaining
// on every read would fill the output with the same sentence while the page
// polls.
func (u *ui) reportUnplaced(c plugin.Contribution) {
	key := c.Owner + ":" + c.Name + ":" + c.Place

	u.mu.Lock()
	if u.unplaced == nil {
		u.unplaced = map[string]bool{}
	}
	alreadySaid := u.unplaced[key]
	u.unplaced[key] = true
	u.mu.Unlock()

	if alreadySaid {
		return
	}

	who := c.Owner
	if who == "" {
		who = "a plugin"
	}
	log.Printf("web: %s offered %q for place %q, which this interface does not have — not drawn", who, c.Name, c.Place)
}

// --- the plugin tree -------------------------------------------------------

// pluginsResponse is what a settings screen needs to draw the plugin list.
type pluginsResponse struct {
	Mounted   []string `json:"mounted"`
	Available []string `json:"available"`
}

// --- approvals ------------------------------------------------------------

// Approve asks the browser and waits for the answer. Implementing this is what
// makes a gated tool call stop and ask rather than simply fail.
func (u *ui) Approve(ctx context.Context, prompt string) guard.Approval {
	u.mu.Lock()
	u.nextID++
	id := strconv.Itoa(u.nextID)
	reply := make(chan guard.Approval, 1)
	u.pending[id] = reply
	u.mu.Unlock()

	defer func() {
		u.mu.Lock()
		delete(u.pending, id)
		u.mu.Unlock()
	}()

	u.broadcast(event{Kind: "approval", ID: id, Text: prompt})

	select {
	case answer := <-reply:
		return answer
	case <-ctx.Done():
		// Nobody answered in time. Refusing is the safe direction.
		return guard.Unavailable
	}
}

type approveRequest struct {
	ID    string `json:"id"`
	Allow bool   `json:"allow"`
}

func (u *ui) approve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "web: POST only", http.StatusMethodNotAllowed)
		return
	}

	var req approveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "web: bad request", http.StatusBadRequest)
		return
	}

	u.mu.Lock()
	reply, ok := u.pending[req.ID]
	u.mu.Unlock()

	if !ok {
		http.Error(w, "web: no such question", http.StatusNotFound)
		return
	}

	answer := guard.Denied
	if req.Allow {
		answer = guard.Approved
	}
	reply <- answer

	w.WriteHeader(http.StatusAccepted)
}

// --- commands -------------------------------------------------------------

// registerCommands puts the interface's own commands into the registry. These
// are the settings: /model, /workspace and /source change things live, and
// anything else registered shows up in /help too.
func (u *ui) registerCommands() {
	if u.commands == nil {
		u.commands = commands.New()
	}

	defs := []commands.Command{
		{
			Name:        "help",
			Description: "list the commands",
			Run: func([]string) (string, error) {
				var b strings.Builder
				b.WriteString("Commands:")
				for _, c := range u.commands.All() {
					usage := c.Usage
					if usage == "" {
						usage = "/" + c.Name
					}
					fmt.Fprintf(&b, "\n  %-22s %s", usage, c.Description)
				}
				return b.String(), nil
			},
		},
		{
			Name:        "model",
			Usage:       "/model [name]",
			Description: "show or change the model",
			Run: func(args []string) (string, error) {
				if len(args) == 0 {
					return "model: " + u.model(), nil
				}
				u.rt.Model = args[0]
				if u.rt.Settings != nil {
					u.rt.Settings.SetModel(args[0])
				}
				u.broadcast(event{Kind: "state"})
				u.broadcast(event{Kind: "settings"})
				return "model is now " + args[0], nil
			},
		},
		{
			Name:        "workspace",
			Usage:       "/workspace [dir ...]",
			Description: "show or change where the agent may write",
			Run: func(args []string) (string, error) {
				if u.rt.Workspace == nil {
					return "no workspace is configured", nil
				}
				if len(args) == 0 {
					dirs := u.rt.Workspace.Dirs()
					if len(dirs) == 0 {
						return "no workspace is configured", nil
					}
					return "workspace:\n  " + strings.Join(dirs, "\n  "), nil
				}
				if err := u.rt.Workspace.Set(args...); err != nil {
					return "", err
				}
				u.broadcast(event{Kind: "state"})
				return "workspace is now:\n  " + strings.Join(u.rt.Workspace.Dirs(), "\n  "), nil
			},
		},
		{
			Name:        "source",
			Usage:       "/source [dir]",
			Description: "show or change where the harness's own source lives",
			Run: func(args []string) (string, error) {
				if u.rt.Guard == nil {
					return "no guard is configured", nil
				}
				if len(args) == 0 {
					src := u.rt.Guard.SourceOf()
					if src == "" {
						return "no source is set, so editing your own code is not gated", nil
					}
					return "source: " + src, nil
				}
				u.rt.Guard.SetSource(args[0])
				return "source is now " + args[0], nil
			},
		},
		{
			Name:        "new",
			Description: "start a new session",
			Run: func([]string) (string, error) {
				u.rt.Agent.Reset()
				u.mu.Lock()
				u.current = ""
				u.mu.Unlock()
				u.broadcast(event{Kind: "cleared"})
				u.broadcast(event{Kind: "sessions"})
				return "Started a new session.", nil
			},
		},
		{
			Name:        "sessions",
			Description: "list saved sessions",
			Run: func([]string) (string, error) {
				list, err := u.store.List()
				if err != nil {
					return "", err
				}
				if len(list) == 0 {
					return "No saved sessions yet.", nil
				}
				var b strings.Builder
				for i, s := range list {
					if i > 0 {
						b.WriteString("\n")
					}
					fmt.Fprintf(&b, "%s  %s  (%d messages)", s.ID, s.Title, len(s.Messages))
				}
				return b.String(), nil
			},
		},
		{
			Name:        "resume",
			Usage:       "/resume <id>",
			Description: "continue a saved session",
			Run: func(args []string) (string, error) {
				if len(args) == 0 {
					return "", fmt.Errorf("usage: /resume <id>")
				}
				if err := u.rt.Agent.Resume(args[0]); err != nil {
					return "", err
				}
				u.mu.Lock()
				u.current = args[0]
				u.mu.Unlock()
				u.broadcast(event{Kind: "history"})
				return "Resumed " + args[0] + ".", nil
			},
		},
		{
			Name:        "pin",
			Usage:       "/pin [id]",
			Description: "pin the current or named session",
			Run:         func(args []string) (string, error) { return u.flagSession(args, "pin") },
		},
		{
			Name:        "archive",
			Usage:       "/archive [id]",
			Description: "archive the current or named session",
			Run:         func(args []string) (string, error) { return u.flagSession(args, "archive") },
		},
		{
			Name:        "delete",
			Usage:       "/delete <id>",
			Description: "delete a session",
			Run: func(args []string) (string, error) {
				if len(args) == 0 {
					return "", fmt.Errorf("usage: /delete <id>")
				}
				if err := u.store.Delete(args[0]); err != nil {
					return "", err
				}
				u.broadcast(event{Kind: "sessions"})
				return "Deleted session " + args[0] + ".", nil
			},
		},
	}

	for _, c := range defs {
		_ = u.commands.Register(c)
	}
}

// flagSession pins or archives a session, named or the current one.
func (u *ui) flagSession(args []string, what string) (string, error) {
	u.mu.Lock()
	id := u.current
	u.mu.Unlock()

	if len(args) > 0 {
		id = args[0]
	}
	if id == "" {
		return "", fmt.Errorf("no session yet — send a message first, or name one: /%s <id>", what)
	}

	var err error
	if what == "pin" {
		err = u.store.SetPinned(id, true)
	} else {
		err = u.store.SetArchived(id, true)
	}
	if err != nil {
		return "", err
	}

	u.broadcast(event{Kind: "sessions"})
	return fmt.Sprintf("Session %s %sed.", id, what), nil
}

type commandRequest struct {
	Line string `json:"line"`
}

func (u *ui) command(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "web: POST only", http.StatusMethodNotAllowed)
		return
	}

	var req commandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "web: bad request", http.StatusBadRequest)
		return
	}

	name, args := commands.Split(req.Line)
	if name == "" {
		http.Error(w, "web: empty command", http.StatusBadRequest)
		return
	}

	c, ok := u.commands.Get(name)
	if !ok {
		u.broadcast(event{Kind: "command", Text: "unknown command: /" + name + " (try /help)"})
		w.WriteHeader(http.StatusAccepted)
		return
	}

	out, err := c.Run(args)
	if err != nil {
		u.broadcast(event{Kind: "command", Text: "error: " + err.Error()})
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if out != "" {
		u.broadcast(event{Kind: "command", Text: out})
	}
	w.WriteHeader(http.StatusAccepted)
}

// --- connected clients ----------------------------------------------------

func (u *ui) addClient(ch chan event) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.clients[ch] = struct{}{}
}

func (u *ui) removeClient(ch chan event) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.clients, ch)
}

// broadcast sends an event to every open browser.
func (u *ui) broadcast(ev event) {
	u.mu.Lock()
	clients := make([]chan event, 0, len(u.clients))
	for ch := range u.clients {
		clients = append(clients, ch)
	}
	u.mu.Unlock()

	for _, ch := range clients {
		u.push(ch, ev)
	}
}

// push never blocks: a browser that has stopped reading loses events rather
// than holding the agent up.
func (u *ui) push(ch chan event, ev event) {
	select {
	case ch <- ev:
	default:
	}
}

// --- modes ----------------------------------------------------------------

// modeRow is one mode, for the control that switches them.
// --- providers ------------------------------------------------------------

// router is where model calls go, when whatever is in use can say what else
// there is and change which one is being used. A single-endpoint provider
// cannot, and that isn't an error: the page gets an empty list and no control.
type router = plugin.Router

// providerRow is one endpoint, for the control that switches them.
type providerRow struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Kind    string `json:"kind,omitempty"`
	KeyFrom string `json:"keyFrom,omitempty"`
	Current bool   `json:"current"`
}

// providersGet reports where model calls can go, and which one is in use.
//
// It reports names, urls, and where a key comes from — never a key. The entry
// type keeps the key itself out of anything that can be serialised, so a page
// cannot be handed a secret even by accident.
func (u *ui) providersGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	sw, ok := u.rt.Provider.(router)
	if !ok {
		_, _ = w.Write([]byte("[]"))
		return
	}

	all := sw.List()

	rows := make([]providerRow, 0, len(all))
	for _, e := range all {
		rows = append(rows, providerRow{
			Name:    e.Name,
			URL:     e.URL,
			KeyFrom: e.KeyFrom,
			Current: e.Current,
		})
	}
	_ = json.NewEncoder(w).Encode(rows)
}

type providerSetRequest struct {
	Name string `json:"name"`
}

// providersSet changes which endpoint model calls go to. It takes effect on the
// next call rather than the next restart.
//
// A name the harness doesn't have is a bad request, because the request asked
// for something that isn't a thing. Anything else that goes wrong is ours.
func (u *ui) providersSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "web: POST only", http.StatusMethodNotAllowed)
		return
	}

	sw, ok := u.rt.Provider.(router)
	if !ok {
		http.Error(w, "web: the provider in use cannot be switched", http.StatusServiceUnavailable)
		return
	}

	var req providerSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "web: bad request", http.StatusBadRequest)
		return
	}

	known := false
	for _, e := range sw.List() {
		if e.Name == req.Name {
			known = true
			break
		}
	}
	if !known {
		http.Error(w, "web: no such provider: "+req.Name, http.StatusBadRequest)
		return
	}

	if err := sw.Choose(req.Name); err != nil {
		http.Error(w, "web: "+err.Error(), http.StatusInternalServerError)
		return
	}

	u.broadcast(event{Kind: "providers"})
	u.broadcast(event{Kind: "state"})
	w.WriteHeader(http.StatusAccepted)
}

type providerAddRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`

	// Key is either the name of an environment variable to read the key from,
	// or the key itself. The screen asks for the variable name, because a key
	// written down is a key written down.
	Key string `json:"key"`

	// Kind is the wire format the endpoint speaks. Empty means "openai", which
	// is what everything that existed before there was a choice means.
	Kind string `json:"kind"`
}

// providersAdd makes an endpoint available. Adding one already there replaces
// it, so a url typed wrongly is a correction rather than a fight.
func (u *ui) providersAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "web: POST only", http.StatusMethodNotAllowed)
		return
	}

	sw, ok := u.rt.Provider.(router)
	if !ok {
		http.Error(w, "web: the provider in use cannot be changed", http.StatusServiceUnavailable)
		return
	}

	var req providerAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "web: bad request", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.URL == "" {
		http.Error(w, "web: a provider needs a name and a url", http.StatusBadRequest)
		return
	}

	if err := sw.Add(plugin.ProviderSpec{Name: req.Name, URL: req.URL, Key: req.Key, Kind: req.Kind}); err != nil {
		http.Error(w, "web: "+err.Error(), http.StatusInternalServerError)
		return
	}

	u.broadcast(event{Kind: "providers"})
	u.broadcast(event{Kind: "state"})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(providerRow{Name: req.Name, URL: req.URL, Kind: req.Kind})
}

type providerRemoveRequest struct {
	Name string `json:"name"`
}

// providersRemove takes an endpoint out. The name is checked against the ones
// the harness has before the removal is attempted, so a name that isn't a
// provider is a bad request rather than our fault.
func (u *ui) providersRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "web: POST only", http.StatusMethodNotAllowed)
		return
	}

	sw, ok := u.rt.Provider.(router)
	if !ok {
		http.Error(w, "web: the provider in use cannot be changed", http.StatusServiceUnavailable)
		return
	}

	var req providerRemoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "web: bad request", http.StatusBadRequest)
		return
	}

	known := false
	for _, e := range sw.List() {
		if e.Name == req.Name {
			known = true
			break
		}
	}
	if !known {
		http.Error(w, "web: no such provider: "+req.Name, http.StatusBadRequest)
		return
	}

	if err := sw.Remove(req.Name); err != nil {
		http.Error(w, "web: "+err.Error(), http.StatusBadRequest)
		return
	}

	u.broadcast(event{Kind: "providers"})
	u.broadcast(event{Kind: "state"})
	w.WriteHeader(http.StatusAccepted)
}

// ensure the interface really is a frontend and a plugin, and that the object
// the harness runs is also the one that can ask a person.
var (
	_ interface{ Approver() guard.Approver } = (*ui)(nil)
	_ core.Plugin                            = (*Plugin)(nil)
	_ frontend.Frontend                      = (*ui)(nil)
)

// promptGet shows what the model is told before anything else, part by part.
//
// It exists because "why did it know that" should have an answer you can look at:
// a part that is in the prompt names itself, and a part that is missing does not
// appear at all — which is the same information read the other way.
func (u *ui) promptGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "prompt: GET only", http.StatusMethodNotAllowed)
		return
	}

	parts := []prompt.Part{}
	if u.prompt != nil {
		parts = u.prompt.Parts()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"parts": parts,
		"count": len(parts),
	})
}

// promptFull is the whole request as the model receives it: the system text, then
// every tool it may call with the schema each one accepts.
//
// /api/prompt shows the words only, because that is all the seam assembles. This
// exists because "the prompt" at the API layer is wider than that, and reading it in
// one place is the difference between debugging a model and guessing at one.
func (u *ui) promptFull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "prompt: GET only", http.StatusMethodNotAllowed)
		return
	}

	system := ""
	parts := []prompt.Part{}
	if u.prompt != nil {
		parts = u.prompt.Parts()
		system = u.prompt.Text()
	}

	type toolView struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Schema      json.RawMessage `json:"schema"`
	}

	calls := []toolView{}
	if u.tools != nil {
		for _, name := range u.tools.Names() {
			tool, ok := u.tools.Get(name)
			if !ok {
				continue
			}
			calls = append(calls, toolView{
				Name:        tool.Name(),
				Description: tool.Description(),
				Schema:      tool.Schema(),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"system": system,
		"parts":  parts,
		"tools":  calls,
		"counts": map[string]int{"parts": len(parts), "tools": len(calls)},
		// The conversation is not included: it belongs to a run, not to the
		// harness, and a page does not need the transcript it is already showing.
	})
}
