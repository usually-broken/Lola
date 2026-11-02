package web

import (
	"testing"

	"github.com/usually-broken/lola/core/frontend"
	"github.com/usually-broken/lola/core/guard"
)

// The harness takes the frontend out of the tree and then asks it whether it
// can approve gated calls. When the answer was hung off the plugin wrapper
// instead of the frontend itself, that check quietly failed, the gate kept its
// refuse-all default, and every gated call failed instead of putting a question
// on screen. This is the check that would have caught it.
func TestTheRunningFrontendCarriesTheApprover(t *testing.T) {
	p := &Plugin{}
	var f frontend.Frontend = p.Frontend()

	if _, ok := f.(interface{ Approver() guard.Approver }); !ok {
		t.Fatal("the frontend the harness holds cannot approve, so gated calls are refused rather than asked about")
	}
}
