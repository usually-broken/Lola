package guard

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

// fixedApprover answers every question the same way.
type fixedApprover struct{ answer Approval }

func (f fixedApprover) Approve(context.Context, string) Approval { return f.answer }

func gatedArg(t *testing.T, dir string) json.RawMessage {
	t.Helper()
	// The config file is gated, so it always raises a question.
	b, err := json.Marshal(map[string]string{"path": filepath.Join(dir, "lola.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestGateRefusesGatedCallWithNoApprover(t *testing.T) {
	dir := t.TempDir()
	g := NewGate(Default(dir, ""), nil)

	ok, reason := g.Check(context.Background(), "write_file", gatedArg(t, dir))
	if ok {
		t.Fatal("a gated call must be refused when there is nobody to ask")
	}
	if reason == "" {
		t.Fatal("a refusal should carry a reason")
	}
}

func TestGateProceedsWhenApproved(t *testing.T) {
	dir := t.TempDir()
	g := NewGate(Default(dir, ""), fixedApprover{answer: Approved})

	if ok, reason := g.Check(context.Background(), "write_file", gatedArg(t, dir)); !ok {
		t.Fatalf("an approved call should proceed, reason=%q", reason)
	}
}

func TestGateStopsWhenDenied(t *testing.T) {
	dir := t.TempDir()
	g := NewGate(Default(dir, ""), fixedApprover{answer: Denied})

	if ok, _ := g.Check(context.Background(), "write_file", gatedArg(t, dir)); ok {
		t.Fatal("a denied call must not proceed")
	}
}

func TestSetApproverSwapsTheAnswer(t *testing.T) {
	dir := t.TempDir()
	g := NewGate(Default(dir, ""), RefuseAll{})

	if ok, _ := g.Check(context.Background(), "write_file", gatedArg(t, dir)); ok {
		t.Fatal("RefuseAll should refuse")
	}

	g.SetApprover(fixedApprover{answer: Approved})
	if ok, _ := g.Check(context.Background(), "write_file", gatedArg(t, dir)); !ok {
		t.Fatal("after SetApprover, an approving approver should let it through")
	}
}

func TestProtectedIsNeverApprovedAway(t *testing.T) {
	dir := t.TempDir()
	// Even with an approver that says yes to everything, protected stays denied.
	g := NewGate(Default(dir, ""), fixedApprover{answer: Approved})

	args, _ := json.Marshal(map[string]string{"path": filepath.Join(dir, "sessions", "x.json")})
	if ok, _ := g.Check(context.Background(), "write_file", args); ok {
		t.Fatal("a protected path must be denied even with an approving approver")
	}
}
