package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/usually-broken/lola/core/agent"
	"github.com/usually-broken/lola/core/frontend"
	"github.com/usually-broken/lola/core/guard"
	"github.com/usually-broken/lola/core/llm"
	"github.com/usually-broken/lola/core/session"
)

// --- a stand-in agent ------------------------------------------------------

// plainAgent answers without ever calling a tool.
type plainAgent struct{ task string }

func (a *plainAgent) Run(_ context.Context, _ llm.Provider, _, task string) (agent.Result, error) {
	a.task = task
	return agent.Result{Content: "answered: " + task, Steps: 1}, nil
}
func (a *plainAgent) History() []llm.Message { return nil }
func (a *plainAgent) Reset()                 {}
func (a *plainAgent) Resume(string) error    { return nil }

// watchedAgent reports a tool call as it happens, the way the real loop does.
type watchedAgent struct {
	plainAgent
	steps []llm.ToolCall
}

func (a *watchedAgent) RunWatched(_ context.Context, _ llm.Provider, _, task string, onStep func(llm.ToolCall)) (agent.Result, error) {
	call := llm.ToolCall{ID: "1", Name: "run_shell", Arguments: json.RawMessage(`{"command":"ls"}`)}
	onStep(call)
	a.steps = append(a.steps, call)
	a.task = task
	return agent.Result{Content: "ran it", Steps: 2}, nil
}

// failingAgent returns an error, to prove the browser is told.
type failingAgent struct{ plainAgent }

func (a *failingAgent) Run(context.Context, llm.Provider, string, string) (agent.Result, error) {
	return agent.Result{}, errors.New("model exploded")
}

// --- helpers ---------------------------------------------------------------

func newUI(t *testing.T, ag agent.Agent) *ui {
	t.Helper()
	u := &ui{
		rt:      frontend.Runtime{Agent: ag, Model: "test-model", Plugins: []string{"a", "b"}, SessionsDir: t.TempDir()},
		store:   session.NewStore(filepath.Join(t.TempDir(), session.DefaultFile)),
		clients: make(map[chan event]struct{}),
		pending: make(map[string]chan guard.Approval),
	}
	u.registerCommands()
	return u
}

// --- the page --------------------------------------------------------------

func TestThePageIsServed(t *testing.T) {
	u := newUI(t, &plainAgent{})
	rec := httptest.NewRecorder()

	u.index(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"<title>Lola</title>", "--accent:", "/api/events", `data-place="composer"`, "fillPlaces("} {
		if !strings.Contains(body, want) {
			t.Errorf("the page should contain %q", want)
		}
	}
}

func TestThePageIsOnlyAtTheRoot(t *testing.T) {
	u := newUI(t, &plainAgent{})
	rec := httptest.NewRecorder()

	u.index(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestThePageIsEmbedded(t *testing.T) {
	page, err := assets.ReadFile("index.html")
	if err != nil {
		t.Fatalf("the page should be embedded in the binary: %v", err)
	}
	if len(page) == 0 {
		t.Fatal("the embedded page is empty")
	}
}

// --- state and sessions ----------------------------------------------------

func TestStateReportsTheRuntime(t *testing.T) {
	u := newUI(t, &plainAgent{})
	rec := httptest.NewRecorder()

	u.state(rec, httptest.NewRequest(http.MethodGet, "/api/state", nil))

	var got stateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("state should be JSON: %v", err)
	}
	if got.Model != "test-model" {
		t.Errorf("model = %q", got.Model)
	}
	if len(got.Plugins) != 2 {
		t.Errorf("plugins = %v", got.Plugins)
	}
	if got.Busy {
		t.Error("a fresh interface should not be busy")
	}
}

// --- sending ---------------------------------------------------------------

func TestSendNeedsAPost(t *testing.T) {
	u := newUI(t, &plainAgent{})
	rec := httptest.NewRecorder()

	u.send(rec, httptest.NewRequest(http.MethodGet, "/api/send", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestSendRejectsAnEmptyMessage(t *testing.T) {
	u := newUI(t, &plainAgent{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/send", strings.NewReader(`{"text":"   "}`))

	u.send(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSendRejectsAStrayBody(t *testing.T) {
	u := newUI(t, &plainAgent{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/send", strings.NewReader("not json"))

	u.send(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSendRejectsWhileBusy(t *testing.T) {
	u := newUI(t, &plainAgent{})
	u.busy = true

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/send", strings.NewReader(`{"text":"hello"}`))
	u.send(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

// --- the live stream -------------------------------------------------------

// waitFor reads events until one of the wanted kind arrives, or the test times
// out.
func waitFor(t *testing.T, ch chan event, kind string) event {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Kind == kind {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for a %q event", kind)
			return event{}
		}
	}
}

func TestARunStreamsStepsThenTheAnswer(t *testing.T) {
	u := newUI(t, &watchedAgent{})
	u.busy = true // pretend the send already happened

	ch := make(chan event, 32)
	u.addClient(ch)
	defer u.removeClient(ch)

	u.run("do the thing")

	step := waitFor(t, ch, "step")
	if step.Tool != "run_shell" {
		t.Errorf("step tool = %q, want run_shell", step.Tool)
	}
	if !strings.Contains(step.Text, "ls") {
		t.Errorf("step text = %q, should carry the arguments", step.Text)
	}

	answer := waitFor(t, ch, "answer")
	if answer.Text != "ran it" {
		t.Errorf("answer = %q", answer.Text)
	}

	busy := waitFor(t, ch, "busy")
	if busy.Busy {
		t.Error("the run should end with busy false")
	}
}

func TestAnAgentWithoutWatchStillAnswers(t *testing.T) {
	u := newUI(t, &plainAgent{})

	ch := make(chan event, 32)
	u.addClient(ch)
	defer u.removeClient(ch)

	u.run("hello")

	if answer := waitFor(t, ch, "answer"); answer.Text != "answered: hello" {
		t.Errorf("answer = %q", answer.Text)
	}
}

func TestAFailedRunIsReported(t *testing.T) {
	u := newUI(t, &failingAgent{})

	ch := make(chan event, 32)
	u.addClient(ch)
	defer u.removeClient(ch)

	u.run("anything")

	if ev := waitFor(t, ch, "error"); !strings.Contains(ev.Text, "model exploded") {
		t.Errorf("error = %q", ev.Text)
	}
}

func TestABusyRunReportsItself(t *testing.T) {
	u := newUI(t, &plainAgent{})

	ch := make(chan event, 32)
	u.addClient(ch)
	defer u.removeClient(ch)

	u.run("x")

	waitFor(t, ch, "answer")
	if ev := waitFor(t, ch, "busy"); ev.Busy {
		t.Error("busy should be false once the run has finished")
	}
}

func TestBroadcastReachesEveryClient(t *testing.T) {
	u := newUI(t, &plainAgent{})

	a := make(chan event, 4)
	b := make(chan event, 4)
	u.addClient(a)
	u.addClient(b)
	defer u.removeClient(a)
	defer u.removeClient(b)

	u.broadcast(event{Kind: "user", Text: "hi"})

	for i, ch := range []chan event{a, b} {
		select {
		case ev := <-ch:
			if ev.Text != "hi" {
				t.Errorf("client %d got %q", i, ev.Text)
			}
		default:
			t.Errorf("client %d got nothing", i)
		}
	}
}

func TestASlowClientDoesNotBlockTheRun(t *testing.T) {
	u := newUI(t, &plainAgent{})

	// deliberately unbuffered, and nobody reading
	stuck := make(chan event)
	u.addClient(stuck)
	defer u.removeClient(stuck)

	done := make(chan struct{})
	go func() {
		defer close(done)
		u.broadcast(event{Kind: "answer", Text: "should not block"})
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("broadcast blocked on a client that stopped reading")
	}
}

// --- approvals -------------------------------------------------------------

func TestApproveAsksTheBrowserThenTakesTheAnswer(t *testing.T) {
	u := newUI(t, &plainAgent{})

	ch := make(chan event, 8)
	u.addClient(ch)
	defer u.removeClient(ch)

	returned := make(chan guard.Approval, 1)
	go func() { returned <- u.Approve(context.Background(), "write to lola.yaml") }()

	question := waitFor(t, ch, "approval")
	if question.Text != "write to lola.yaml" {
		t.Errorf("question = %q", question.Text)
	}
	if question.ID == "" {
		t.Fatal("a question needs an id so the answer can be matched to it")
	}

	rec := httptest.NewRecorder()
	u.approve(rec, httptest.NewRequest(http.MethodPost, "/api/approve",
		strings.NewReader(`{"id":"`+question.ID+`","allow":true}`)))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}

	select {
	case got := <-returned:
		if got != guard.Approved {
			t.Fatalf("answer = %v, want Approved", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Approve never returned")
	}
}

func TestApproveCarriesADenial(t *testing.T) {
	u := newUI(t, &plainAgent{})

	ch := make(chan event, 8)
	u.addClient(ch)
	defer u.removeClient(ch)

	returned := make(chan guard.Approval, 1)
	go func() { returned <- u.Approve(context.Background(), "run: rm -rf /") }()

	question := waitFor(t, ch, "approval")

	rec := httptest.NewRecorder()
	u.approve(rec, httptest.NewRequest(http.MethodPost, "/api/approve",
		strings.NewReader(`{"id":"`+question.ID+`","allow":false}`)))

	select {
	case got := <-returned:
		if got != guard.Denied {
			t.Fatalf("answer = %v, want Denied", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Approve never returned")
	}
}

func TestApproveRefusesWhenNobodyAnswers(t *testing.T) {
	u := newUI(t, &plainAgent{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// nothing is connected and the context is already dead
	if got := u.Approve(ctx, "anything"); got != guard.Unavailable {
		t.Fatalf("= %v, want Unavailable so the call is refused", got)
	}
}

func TestApproveRejectsAnUnknownQuestion(t *testing.T) {
	u := newUI(t, &plainAgent{})
	rec := httptest.NewRecorder()

	u.approve(rec, httptest.NewRequest(http.MethodPost, "/api/approve",
		strings.NewReader(`{"id":"999","allow":true}`)))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestApproveNeedsAPost(t *testing.T) {
	u := newUI(t, &plainAgent{})
	rec := httptest.NewRecorder()

	u.approve(rec, httptest.NewRequest(http.MethodGet, "/api/approve", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

// --- commands --------------------------------------------------------------

func TestTheSettingsCommandsAreRegistered(t *testing.T) {
	u := newUI(t, &plainAgent{})

	for _, name := range []string{"help", "model", "workspace", "source"} {
		if _, ok := u.commands.Get(name); !ok {
			t.Errorf("/%s should be registered", name)
		}
	}
}

func TestACommandRunsAndIsReported(t *testing.T) {
	u := newUI(t, &plainAgent{})

	ch := make(chan event, 8)
	u.addClient(ch)
	defer u.removeClient(ch)

	rec := httptest.NewRecorder()
	u.command(rec, httptest.NewRequest(http.MethodPost, "/api/command",
		strings.NewReader(`{"line":"/model changed-model"}`)))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if u.rt.Model != "changed-model" {
		t.Errorf("model = %q, and the command should have changed it", u.rt.Model)
	}
	if ev := waitFor(t, ch, "command"); !strings.Contains(ev.Text, "changed-model") {
		t.Errorf("output = %q", ev.Text)
	}
}

func TestHelpListsTheCommands(t *testing.T) {
	u := newUI(t, &plainAgent{})

	ch := make(chan event, 8)
	u.addClient(ch)
	defer u.removeClient(ch)

	rec := httptest.NewRecorder()
	u.command(rec, httptest.NewRequest(http.MethodPost, "/api/command",
		strings.NewReader(`{"line":"/help"}`)))

	ev := waitFor(t, ch, "command")
	for _, want := range []string{"/model", "/workspace", "/source"} {
		if !strings.Contains(ev.Text, want) {
			t.Errorf("/help should list %s, got:\n%s", want, ev.Text)
		}
	}
}

func TestAnUnknownCommandIsReported(t *testing.T) {
	u := newUI(t, &plainAgent{})

	ch := make(chan event, 8)
	u.addClient(ch)
	defer u.removeClient(ch)

	rec := httptest.NewRecorder()
	u.command(rec, httptest.NewRequest(http.MethodPost, "/api/command",
		strings.NewReader(`{"line":"/nonsense"}`)))

	if ev := waitFor(t, ch, "command"); !strings.Contains(ev.Text, "unknown command") {
		t.Errorf("output = %q", ev.Text)
	}
}

func TestCommandNeedsAPost(t *testing.T) {
	u := newUI(t, &plainAgent{})
	rec := httptest.NewRecorder()

	u.command(rec, httptest.NewRequest(http.MethodGet, "/api/command", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestCommandRejectsAnEmptyLine(t *testing.T) {
	u := newUI(t, &plainAgent{})
	rec := httptest.NewRecorder()

	u.command(rec, httptest.NewRequest(http.MethodPost, "/api/command",
		strings.NewReader(`{"line":"   "}`)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// --- the plugin itself -----------------------------------------------------

func TestSetAddrChangesWhereItListens(t *testing.T) {
	p := &Plugin{addr: DefaultAddr}
	p.SetAddr("127.0.0.1:9999")

	if p.addr != "127.0.0.1:9999" {
		t.Fatalf("addr = %q", p.addr)
	}
}

func TestThePluginHandsOutAFrontend(t *testing.T) {
	p := &Plugin{addr: DefaultAddr}
	fe := p.Frontend()

	if fe == nil {
		t.Fatal("Frontend() should not be nil")
	}
	if _, ok := fe.(*ui); !ok {
		t.Fatalf("Frontend() returned %T, want *ui", fe)
	}
}

func TestTheDefaultAddressIsLoopback(t *testing.T) {
	if !strings.HasPrefix(DefaultAddr, "127.0.0.1:") {
		t.Fatalf("DefaultAddr = %q, and it must be loopback so the interface is not on the network", DefaultAddr)
	}
}
