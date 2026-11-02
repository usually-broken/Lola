package agentloop

import (
	"testing"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/session"
)

// keeperTree is a tree holding one plugin and nothing else: enough to answer the
// single question the loop asks it.
type keeperTree struct{ held core.Plugin }

func (t *keeperTree) Mounted() []string {
	if t.held == nil {
		return nil
	}
	return []string{"sessions"}
}

func (t *keeperTree) Available() []string { return []string{"sessions"} }

func (t *keeperTree) Get(name string) (core.Plugin, bool) {
	if t.held == nil || name != "sessions" {
		return nil, false
	}
	return t.held, true
}

func (t *keeperTree) Mount(string) error   { return nil }
func (t *keeperTree) Unmount(string) error { return nil }

// aKeeper is the smallest thing that can hand out a store.
type aKeeper struct{ store *session.Store }

func (k aKeeper) Name() string                 { return "sessions" }
func (k aKeeper) Setup() (func() error, error) { return func() error { return nil }, nil }
func (k aKeeper) Sessions() *session.Store     { return k.store }

// The loop does not build a store of its own any more: it asks the tree, so the
// loop and the interface the person is looking at write the same files.
func TestTheLoopUsesTheStoreTheTreeOffers(t *testing.T) {
	store := session.NewStore(t.TempDir())
	p := &Plugin{plugins: &keeperTree{held: aKeeper{store: store}}}

	if got := p.sessionStore(); got != store {
		t.Fatal("the loop built its own store instead of using the one the tree offers")
	}
	// Asked once, remembered after, so a long run is not looking it up per step.
	if got := p.sessionStore(); got != store {
		t.Fatal("asking twice did not return the same store")
	}
}

// A config with nothing keeping sessions means this loop keeps its memory in
// memory. That is not an error; it just has to say so when asked to resume.
func TestWithNothingOfferingAStoreTheLoopDoesNotPersist(t *testing.T) {
	p := &Plugin{plugins: &keeperTree{}}

	if got := p.sessionStore(); got != nil {
		t.Fatalf("the loop found a store where there is none: %v", got)
	}
	if err := p.Resume("nonsense"); err == nil {
		t.Fatal("resuming with no store anywhere should say so rather than pretend")
	}
}
