package tui

import (
	"context"

	"github.com/usually-broken/lola/core/guard"
)

// approvalRequest is one question for the person at the keyboard, with a
// channel to carry their answer back.
type approvalRequest struct {
	prompt string
	reply  chan guard.Approval
}

// approver is the guard's consent seam for the TUI.
//
// The guard calls Approve from the agent's goroutine during a tool call and
// blocks. This hands the question to the UI over a channel and waits for the
// answer. If the context ends first, it declines — safe by default.
type approver struct {
	requests chan approvalRequest
}

func newApprover() *approver {
	return &approver{requests: make(chan approvalRequest)}
}

// Approve implements guard.Approver.
func (a *approver) Approve(ctx context.Context, prompt string) guard.Approval {
	req := approvalRequest{prompt: prompt, reply: make(chan guard.Approval, 1)}

	select {
	case a.requests <- req:
	case <-ctx.Done():
		return guard.Unavailable
	}

	select {
	case answer := <-req.reply:
		return answer
	case <-ctx.Done():
		return guard.Unavailable
	}
}

var _ guard.Approver = (*approver)(nil)
