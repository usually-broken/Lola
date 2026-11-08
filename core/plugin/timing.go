package plugin

import (
	"sync"
	"time"
)

// Turn is one request to the model, timed.
//
// It exists because nothing else can answer "how long until the first word" —
// the answer lives inside the loop, and the loop is a plugin like any other, so
// it reports here rather than being asked.
type Turn struct {
	// Started is when the request was sent.
	Started time.Time

	// First is when the first piece of the answer arrived. Zero when the
	// adapter does not stream, because then there is no first piece.
	First time.Time

	// Done is when the answer finished.
	Done time.Time

	// Chars is how much text came back, which is what a rate is made of.
	Chars int
}

// FirstWait is how long it took before anything appeared. False when unknown.
func (t Turn) FirstWait() (time.Duration, bool) {
	if t.First.IsZero() || t.Started.IsZero() {
		return 0, false
	}
	return t.First.Sub(t.Started), true
}

// Duration is how long the whole answer took.
func (t Turn) Duration() time.Duration {
	if t.Started.IsZero() || t.Done.IsZero() {
		return 0
	}
	return t.Done.Sub(t.Started)
}

// CharsPerSecond is how much text arrived per second once it started. False
// when there is not enough of a measurement to divide by.
func (t Turn) CharsPerSecond() (float64, bool) {
	wait, ok := t.FirstWait()
	if !ok || t.Chars == 0 {
		return 0, false
	}
	elapsed := t.Done.Sub(t.First)
	if elapsed <= 0 {
		return 0, false
	}
	_ = wait
	return float64(t.Chars) / elapsed.Seconds(), true
}

var turns struct {
	mu   sync.RWMutex
	subs []func(Turn)
}

// OnTurn listens for finished turns. Said once at setup; there is no unsubscribe
// because a plugin that has gone away is not called again.
func OnTurn(fn func(Turn)) {
	if fn == nil {
		return
	}
	turns.mu.Lock()
	turns.subs = append(turns.subs, fn)
	turns.mu.Unlock()
}

// ReportTurn tells everyone listening. Called by the loop, never by a screen.
func ReportTurn(t Turn) {
	turns.mu.RLock()
	subs := append([]func(Turn){}, turns.subs...)
	turns.mu.RUnlock()
	for _, fn := range subs {
		fn(t)
	}
}
