package guard

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
)

// asking is a policy that puts everything to the person, so the approver is
// always consulted.
type asking struct{}

func (asking) Check(string, json.RawMessage) Decision { return Ask }

// countingApprover answers the same way every time and counts how often it was
// asked.
type countingApprover struct {
	asked  int32
	answer Approval
}

func (c *countingApprover) Approve(context.Context, string) Approval {
	atomic.AddInt32(&c.asked, 1)
	return c.answer
}

// A refusal is remembered. Once the person has answered, the same request is not
// put to them again — the model is told the answer stands instead.
func TestADeclineIsNotAskedTwice(t *testing.T) {
	person := &countingApprover{answer: Denied}
	g := NewGate(asking{}, person)
	args := json.RawMessage(`{"command":"ls -R /Users/someone"}`)

	allowed, reason := g.Check(context.Background(), "run_shell", args)
	if allowed {
		t.Fatal("a declined call was allowed")
	}
	if !strings.Contains(reason, "declined") {
		t.Fatalf("the refusal does not say it was declined: %q", reason)
	}
	// It has to name the person. A model that reads a refusal as an environment
	// restriction writes a summary about blockers and gives up, in front of
	// somebody who could approve something else straight away.
	if !strings.Contains(reason, "the person you asked") {
		t.Fatalf("the refusal does not say a person said no: %q", reason)
	}
	if !strings.Contains(reason, "not the environment") {
		t.Fatalf("the refusal does not rule out the environment: %q", reason)
	}
	if !strings.Contains(reason, "ask for something else") {
		t.Fatalf("the refusal leaves the model nowhere to go: %q", reason)
	}

	allowed, second := g.Check(context.Background(), "run_shell", args)
	if allowed {
		t.Fatal("the same call was allowed the second time")
	}
	if got := atomic.LoadInt32(&person.asked); got != 1 {
		t.Fatalf("the person was asked %d times, want once", got)
	}
	if !strings.Contains(second, "a moment ago") {
		t.Fatalf("the second refusal does not say the answer already stands: %q", second)
	}
}

// Saying no to one thing does not block everything else: the person is only
// spared the identical request.
func TestADeclineIsRememberedForThatRequestOnly(t *testing.T) {
	person := &countingApprover{answer: Denied}
	g := NewGate(asking{}, person)

	if allowed, _ := g.Check(context.Background(), "run_shell", json.RawMessage(`{"command":"ls -R /"}`)); allowed {
		t.Fatal("a declined call was allowed")
	}

	// A different command is still a question worth asking.
	if allowed, _ := g.Check(context.Background(), "run_shell", json.RawMessage(`{"command":"pwd"}`)); allowed {
		t.Fatal("a different command was allowed without being asked about")
	}
	if got := atomic.LoadInt32(&person.asked); got != 2 {
		t.Fatalf("the person was asked %d times, want twice — a second, different request should still be asked", got)
	}

	// And a different tool is not the same request, even with similar arguments.
	if allowed, _ := g.Check(context.Background(), "write_file", json.RawMessage(`{"path":"notes.txt"}`)); allowed {
		t.Fatal("a write was allowed without being asked about")
	}
	if got := atomic.LoadInt32(&person.asked); got != 3 {
		t.Fatalf("the person was asked %d times, want three", got)
	}
}

// An approval is not recorded as a refusal, so the same call can be approved
// again later in the same run.
func TestAnApprovalIsNotRememberedAsADecline(t *testing.T) {
	person := &countingApprover{answer: Approved}
	g := NewGate(asking{}, person)

	args := json.RawMessage(`{"command":"ls"}`)
	for i := 0; i < 2; i++ {
		if allowed, reason := g.Check(context.Background(), "run_shell", args); !allowed {
			t.Fatalf("an approved call was refused: %s", reason)
		}
	}
	if got := atomic.LoadInt32(&person.asked); got != 2 {
		t.Fatalf("the person was asked %d times, want twice", got)
	}
}

// With nobody to ask, the refusal is unchanged and nothing is remembered as a
// decline — there was no person to say no.
func TestNobodyToAskIsNotADecline(t *testing.T) {
	g := NewGate(asking{}, nil)
	args := json.RawMessage(`{"command":"ls"}`)

	for i := 0; i < 2; i++ {
		allowed, reason := g.Check(context.Background(), "run_shell", args)
		if allowed {
			t.Fatal("a call with nobody to ask was allowed")
		}
		if strings.Contains(reason, "declined") {
			t.Fatalf("a missing approver was reported as a decline: %q", reason)
		}
	}
}
