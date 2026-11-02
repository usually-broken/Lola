package tui

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/usually-broken/lola/core/guard"
)

// --- the approver ---------------------------------------------------------

func TestApproverCarriesTheQuestionAndAnswer(t *testing.T) {
	a := newApprover()

	done := make(chan guard.Approval, 1)
	go func() {
		done <- a.Approve(context.Background(), "run: rm -rf /tmp/x")
	}()

	req := <-a.requests // the UI side picks up the question
	if req.prompt != "run: rm -rf /tmp/x" {
		t.Fatalf("prompt = %q", req.prompt)
	}

	req.reply <- guard.Approved
	if got := <-done; got != guard.Approved {
		t.Fatalf("Approve returned %v, want Approved", got)
	}
}

func TestApproverDeclinesWhenContextEnds(t *testing.T) {
	a := newApprover()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if got := a.Approve(ctx, "anything"); got != guard.Unavailable {
		t.Fatalf("Approve with a dead context = %v, want Unavailable", got)
	}
}

// --- the prompt in the UI -------------------------------------------------

func pendingUI(t *testing.T) (*ui, chan guard.Approval) {
	t.Helper()
	u := newTestUI(t)
	reply := make(chan guard.Approval, 1)
	u.pending = &approvalRequest{prompt: "write to lola.yaml", reply: reply}
	return u, reply
}

func TestApprovalPromptIsShown(t *testing.T) {
	u, _ := pendingUI(t)
	view := u.View()
	if !strings.Contains(view, "approve?") {
		t.Errorf("the footer should ask for approval:\n%s", view)
	}
	if !strings.Contains(view, "write to lola.yaml") {
		t.Errorf("the footer should show the request:\n%s", view)
	}
	if !strings.Contains(view, "[y] yes") {
		t.Errorf("the footer should show the keys:\n%s", view)
	}
}

func TestYApproves(t *testing.T) {
	u, reply := pendingUI(t)

	u.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})

	if got := <-reply; got != guard.Approved {
		t.Fatalf("y sent %v, want Approved", got)
	}
	if u.pending != nil {
		t.Fatal("the question should clear after answering")
	}
}

func TestNDenies(t *testing.T) {
	u, reply := pendingUI(t)

	u.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})

	if got := <-reply; got != guard.Denied {
		t.Fatalf("n sent %v, want Denied", got)
	}
	if u.pending != nil {
		t.Fatal("the question should clear after answering")
	}
}

func TestOtherKeysDoNotDecide(t *testing.T) {
	u, reply := pendingUI(t)

	u.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

	select {
	case got := <-reply:
		t.Fatalf("a stray key answered the question: %v", got)
	default:
	}
	if u.pending == nil {
		t.Fatal("the question should still be pending")
	}
}

func TestApprovalMsgRaisesThePrompt(t *testing.T) {
	u := newTestUI(t)
	reply := make(chan guard.Approval, 1)

	u.Update(approvalMsg{req: approvalRequest{prompt: "read secrets", reply: reply}})

	if u.pending == nil {
		t.Fatal("an approval message should set the pending question")
	}
	if !strings.Contains(u.content.String(), "needs approval") {
		t.Errorf("the transcript should record the request:\n%s", u.content.String())
	}
}

// TestFrontendExposesAnApprover guards the wiring: the harness reaches the
// frontend (not the plugin) for the approver, so if this is missing the guard
// silently falls back to refusing everything.
func TestFrontendExposesAnApprover(t *testing.T) {
	p := &Plugin{approver: newApprover()}
	fe := p.Frontend()

	ap, ok := fe.(interface{ Approver() guard.Approver })
	if !ok {
		t.Fatal("the TUI frontend must expose Approver(), or the guard never asks")
	}
	if ap.Approver() == nil {
		t.Fatal("the exposed approver must not be nil")
	}
}

// TestGateAndApproverEndToEnd wires the real gate to the real approver and
// proves the chain: the gate blocks on a gated call, the question reaches the
// approver, and the answer lets the call proceed.
func TestGateAndApproverEndToEnd(t *testing.T) {
	dir := t.TempDir()
	ap := newApprover()
	gate := guard.NewGate(guard.Default(dir, ""), ap)

	args, _ := json.Marshal(map[string]string{"path": filepath.Join(dir, "lola.yaml")})

	proceeded := make(chan bool, 1)
	go func() {
		ok, _ := gate.Check(context.Background(), "write_file", args)
		proceeded <- ok
	}()

	req := <-ap.requests
	if !strings.Contains(req.prompt, "lola.yaml") {
		t.Fatalf("the question should name the file, got %q", req.prompt)
	}
	req.reply <- guard.Approved

	if ok := <-proceeded; !ok {
		t.Fatal("the gated call should proceed once approved")
	}
}

// TestGateEndToEndDenied proves the other direction: a denial stops the call.
func TestGateEndToEndDenied(t *testing.T) {
	dir := t.TempDir()
	ap := newApprover()
	gate := guard.NewGate(guard.Default(dir, ""), ap)

	args, _ := json.Marshal(map[string]string{"path": filepath.Join(dir, "lola.yaml")})

	proceeded := make(chan bool, 1)
	go func() {
		ok, _ := gate.Check(context.Background(), "write_file", args)
		proceeded <- ok
	}()

	req := <-ap.requests
	req.reply <- guard.Denied

	if ok := <-proceeded; ok {
		t.Fatal("a denied call must not proceed")
	}
}
