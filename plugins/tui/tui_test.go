package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/usually-broken/lola/core/agent"
	"github.com/usually-broken/lola/core/commands"
	"github.com/usually-broken/lola/core/frontend"
	"github.com/usually-broken/lola/core/guard"
	"github.com/usually-broken/lola/core/llm"
	"github.com/usually-broken/lola/core/session"
	"github.com/usually-broken/lola/core/workspace"
)

// newTestUI builds a UI sized like a real terminal, so View can be rendered.
func newTestUI(t *testing.T) *ui {
	t.Helper()
	dir := t.TempDir()
	u := &ui{}
	u.rt = frontend.Runtime{
		Plugins:     []string{"greet", "openai", "agent_loop", "read_file", "write_file", "run_shell", "tui"},
		Model:       "gpt-oss:20b-cloud",
		ConfigPath:  "lola.yaml",
		SessionsDir: dir,
	}
	u.store = session.NewStore(filepath.Join(dir, session.DefaultFile))
	// The interface's commands, exactly as Run would register them.
	u.commands = commands.New()
	u.registerCommands()
	u.spin = spinner.New()
	u.spin.Spinner = spinner.Dot

	u.viewport = viewport.New(80, 20)
	u.input = textarea.New()
	u.input.Placeholder = "Ask Lola to do something…"
	u.input.ShowLineNumbers = false
	u.input.Focus()
	u.inputOK = true

	u.append("welcome")

	m, _ := u.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m.(*ui)
}

func TestViewShowsHeaderStatusAndInput(t *testing.T) {
	u := newTestUI(t)
	view := u.View()

	for _, want := range []string{"L O L A", "agent harness", "model:", "gpt-oss:20b-cloud", "plugins:", "config:", "sessions:", "Ask Lola"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() missing %q\n---\n%s", want, view)
		}
	}
}

func TestViewShowsEveryMountedPlugin(t *testing.T) {
	u := newTestUI(t)
	view := u.View()
	for _, name := range u.rt.Plugins {
		if !strings.Contains(view, name) {
			t.Errorf("View() missing plugin %q", name)
		}
	}
}

func TestTranscriptIsScrollable(t *testing.T) {
	u := newTestUI(t)

	// Fill the transcript well past the viewport height.
	for i := 0; i < 200; i++ {
		u.append("line content here")
	}
	if !strings.Contains(u.viewport.View(), "line content here") {
		t.Fatal("viewport should show transcript content")
	}
	if u.viewport.AtTop() && u.viewport.ScrollPercent() == 0 {
		t.Fatal("after appending past the height, the viewport should not be stuck at the top")
	}

	// Scrolling up must reveal earlier content: the whole point of the fix.
	u.viewport.GotoTop()
	if got := u.viewport.YOffset; got != 0 {
		t.Fatalf("after GotoTop, offset = %d, want 0", got)
	}
	u.viewport.GotoBottom()
	if !u.viewport.AtBottom() {
		t.Fatal("GotoBottom should reach the bottom")
	}
}

func TestEnterSubmitsTask(t *testing.T) {
	u := newTestUI(t)
	u.input.SetValue("do a thing")

	m, cmd := u.Update(tea.KeyMsg{Type: tea.KeyEnter})
	u = m.(*ui)

	if !u.busy {
		t.Fatal("submitting should make the UI busy")
	}
	if cmd == nil {
		t.Fatal("submitting should return a command to run the agent")
	}
	if !strings.Contains(u.content.String(), "do a thing") {
		t.Errorf("the task should appear in the transcript:\n%s", u.content.String())
	}
	if u.input.Value() != "" {
		t.Errorf("input should be cleared after submit, got %q", u.input.Value())
	}
}

func TestEnterWhileBusyIsIgnored(t *testing.T) {
	u := newTestUI(t)
	u.busy = true
	u.input.SetValue("second task")

	m, _ := u.Update(tea.KeyMsg{Type: tea.KeyEnter})
	u = m.(*ui)

	if strings.Contains(u.content.String(), "second task") {
		t.Error("a second task should not be accepted while busy")
	}
}

func TestTabMovesFocusOffTheInput(t *testing.T) {
	u := newTestUI(t)
	if !u.input.Focused() {
		t.Fatal("input should start focused")
	}
	m, _ := u.Update(tea.KeyMsg{Type: tea.KeyTab})
	u = m.(*ui)
	if u.input.Focused() {
		t.Fatal("Tab should move focus off the input so the transcript can scroll")
	}
}

func TestAgentDoneAppendsAnswer(t *testing.T) {
	u := newTestUI(t)
	u.busy = true

	u.Update(agentDoneMsg{res: agent.Result{Content: "the answer", Steps: 2}})

	if u.busy {
		t.Fatal("busy should be false after completion")
	}
	got := u.content.String()
	if !strings.Contains(got, "the answer") {
		t.Errorf("answer not in transcript:\n%s", got)
	}
	if !strings.Contains(got, "2 steps") {
		t.Errorf("step count not in transcript:\n%s", got)
	}
}

func TestAgentDoneShowsError(t *testing.T) {
	u := newTestUI(t)
	u.busy = true

	u.Update(agentDoneMsg{err: errBoom})

	if !strings.Contains(u.content.String(), "boom") {
		t.Errorf("error not shown:\n%s", u.content.String())
	}
}

func TestBusyStateShowsActivity(t *testing.T) {
	u := newTestUI(t)
	u.busy = true
	u.activity = "calling run_shell"

	if !strings.Contains(u.View(), "calling run_shell") {
		t.Error("busy view should show activity")
	}
}

func TestToolCallsAppearInTranscript(t *testing.T) {
	u := newTestUI(t)
	u.busy = true

	// The loop reports each tool call as it happens.
	u.Update(stepMsg{text: "calling run_shell {\"command\":\"date\"}"})

	got := u.content.String()
	if !strings.Contains(got, "run_shell") {
		t.Errorf("transcript should record the tool the agent used:\n%s", got)
	}
	if !strings.Contains(got, "date") {
		t.Errorf("transcript should record the tool arguments:\n%s", got)
	}
}

// --- session commands -----------------------------------------------------

func TestHelpCommandListsCommands(t *testing.T) {
	u := newTestUI(t)
	u.runCommand("/help")

	got := u.content.String()
	for _, cmd := range []string{"/new", "/sessions", "/resume", "/pin", "/archive"} {
		if !strings.Contains(got, cmd) {
			t.Errorf("help should mention %s:\n%s", cmd, got)
		}
	}
}

func TestNewCommandResetsAgent(t *testing.T) {
	u := newTestUI(t)
	u.current = "abc123"
	fake := &fakeAgent{}
	u.rt.Agent = fake

	u.runCommand("/new")

	if !fake.reset {
		t.Error("/new should reset the agent")
	}
	if u.current != "" {
		t.Errorf("current session should be cleared, got %q", u.current)
	}
}

func TestSessionsCommandListsSaved(t *testing.T) {
	u := newTestUI(t)
	if err := u.store.Save(&session.Session{Title: "first chat"}); err != nil {
		t.Fatal(err)
	}

	u.runCommand("/sessions")

	if !strings.Contains(u.content.String(), "first chat") {
		t.Errorf("/sessions should list the saved session:\n%s", u.content.String())
	}
}

func TestResumeCommandOpensPicker(t *testing.T) {
	u := newTestUI(t)
	if err := u.store.Save(&session.Session{Title: "resumable"}); err != nil {
		t.Fatal(err)
	}

	u.runCommand("/resume")

	if !u.pick.open {
		t.Fatal("/resume with saved sessions should open the picker")
	}
	if len(u.pick.sessions) != 1 {
		t.Fatalf("picker has %d sessions, want 1", len(u.pick.sessions))
	}
}

func TestPickerEnterResumesSession(t *testing.T) {
	u := newTestUI(t)
	saved := &session.Session{
		Title:    "old chat",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "remember the word pineapple"}, {Role: llm.RoleAssistant, Content: "noted"}},
	}
	if err := u.store.Save(saved); err != nil {
		t.Fatal(err)
	}
	fake := &fakeAgent{history: saved.Messages}
	u.rt.Agent = fake

	u.runCommand("/resume")
	m, _ := u.Update(tea.KeyMsg{Type: tea.KeyEnter})
	u = m.(*ui)

	if fake.resumedID != saved.ID {
		t.Fatalf("agent resumed %q, want %q", fake.resumedID, saved.ID)
	}
	if u.current != saved.ID {
		t.Fatalf("current = %q, want %q", u.current, saved.ID)
	}
	// The transcript should show the resumed conversation.
	if !strings.Contains(u.content.String(), "pineapple") {
		t.Errorf("resumed history not rendered:\n%s", u.content.String())
	}
}

func TestPickerEscapeCloses(t *testing.T) {
	u := newTestUI(t)
	u.pick = picker{open: true, sessions: []*session.Session{{ID: "x"}}}

	m, _ := u.Update(tea.KeyMsg{Type: tea.KeyEsc})
	u = m.(*ui)
	if u.pick.open {
		t.Fatal("Esc should close the picker")
	}
}

func TestDeleteCommandRemovesSession(t *testing.T) {
	u := newTestUI(t)
	saved := &session.Session{Title: "to delete"}
	if err := u.store.Save(saved); err != nil {
		t.Fatal(err)
	}

	u.runCommand("/delete " + saved.ID)

	if _, err := u.store.Load(saved.ID); err == nil {
		t.Fatal("session should be gone after /delete")
	}
}

func TestUnknownCommandIsReported(t *testing.T) {
	u := newTestUI(t)
	u.runCommand("/nonsense")
	if !strings.Contains(u.content.String(), "unknown command") {
		t.Errorf("unknown command should be reported:\n%s", u.content.String())
	}
}

// TestDiscoveredCommandIsListedAndRuns is the point of the registry: a command
// registered from elsewhere appears in /help and works, with nothing in the
// interface changed to know about it.
func TestDiscoveredCommandIsListedAndRuns(t *testing.T) {
	u := newTestUI(t)

	err := u.commands.Register(commands.Command{
		Name:        "ping",
		Description: "a command from somewhere else",
		Run:         func([]string) (string, error) { return "pong", nil },
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	u.runCommand("/help")
	if !strings.Contains(u.content.String(), "/ping") {
		t.Errorf("/help should list a command it was never told about:\n%s", u.content.String())
	}

	u.runCommand("/ping")
	if !strings.Contains(u.content.String(), "pong") {
		t.Errorf("a discovered command should run:\n%s", u.content.String())
	}
}

// TestSettingsCommands covers /model, /workspace and /source: each shows the
// current value, and each changes it live.
func TestSettingsCommands(t *testing.T) {
	u := newTestUI(t)
	u.rt.Workspace = workspace.MustNew(t.TempDir())
	u.rt.Guard = guard.Default(t.TempDir(), "")

	// /model
	u.runCommand("/model")
	if !strings.Contains(u.content.String(), "gpt-oss:20b-cloud") {
		t.Errorf("/model should show the current model:\n%s", u.content.String())
	}
	u.runCommand("/model other-model")
	if u.rt.Model != "other-model" {
		t.Errorf("model = %q, want other-model", u.rt.Model)
	}

	// /workspace
	newDir := t.TempDir()
	u.runCommand("/workspace " + newDir)
	if _, ok := u.rt.Workspace.Allow(filepath.Join(newDir, "x")); !ok {
		t.Error("/workspace should have changed the roots")
	}

	// /source
	u.runCommand("/source /some/src")
	if got := u.rt.Guard.SourceOf(); got != "/some/src" {
		t.Errorf("source = %q, want /some/src", got)
	}
}

func TestPinNamesCurrentSession(t *testing.T) {
	u := newTestUI(t)
	saved := &session.Session{Title: "pinnable"}
	if err := u.store.Save(saved); err != nil {
		t.Fatal(err)
	}
	u.current = saved.ID

	u.runCommand("/pin")

	got, err := u.store.Load(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Pinned {
		t.Fatal("/pin should pin the current session")
	}
}

// fakeAgent is a stand-in for the mounted loop, so session commands can be
// tested without a model.
type fakeAgent struct {
	reset     bool
	resumedID string
	history   []llm.Message
}

func (f *fakeAgent) Run(context.Context, llm.Provider, string, string) (agent.Result, error) {
	return agent.Result{}, nil
}
func (f *fakeAgent) History() []llm.Message { return f.history }
func (f *fakeAgent) Reset()                 { f.reset = true; f.history = nil }
func (f *fakeAgent) Resume(id string) error {
	f.resumedID = id
	return nil
}

var errBoom = &simpleErr{"boom"}

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }
