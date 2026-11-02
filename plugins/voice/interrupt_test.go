package voice

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The note is single use: it belongs to the turn straight after the interruption. Saying
// it twice would have the model apologising for something nobody remembers.
func TestCutIsExplainedOnce(t *testing.T) {
	p := &Plugin{on: true}

	req := httptest.NewRequest("POST", InterruptPath, strings.NewReader(`{"sentence":"The thing is,","index":3}`))
	p.HandleInterrupt(httptest.NewRecorder(), req)

	first := p.text()
	if !strings.Contains(first, "The thing is,") {
		t.Fatalf("the first turn does not mention the cut: %q", first)
	}
	if !strings.Contains(first, "Do not continue it") {
		t.Fatal("the note does not say what to do instead")
	}

	// The record persists — an answer that stops mid-thought has to stay explainable —
	// but the urgent instruction is said once, to the turn that follows the cut.
	second := p.text()
	if !strings.Contains(second, "The thing is,") {
		t.Fatal("history forgot the interruption on the next turn")
	}
	if strings.Contains(second, "Do not continue it") {
		t.Fatal("the urgent note was repeated after the turn it belonged to")
	}
}

// Voice off means no prompt part at all, interruption or not: text mode is untouched.
func TestCutIsSilentWhenVoiceIsOff(t *testing.T) {
	p := &Plugin{on: false}
	p.HandleInterrupt(httptest.NewRecorder(),
		httptest.NewRequest("POST", InterruptPath, strings.NewReader(`{"sentence":"half a thought"}`)))

	got := p.text()
	if strings.Contains(got, "Voice mode") {
		t.Fatalf("voice off still contributed its instruction: %q", got)
	}
	if !strings.Contains(got, "half a thought") {
		t.Fatal("an interruption was forgotten because the microphone was closed")
	}
}

// An interruption with no sentence in it is not something the model can act on.
func TestInterruptNeedsASentence(t *testing.T) {
	p := &Plugin{on: true}
	rec := httptest.NewRecorder()
	p.HandleInterrupt(rec, httptest.NewRequest("POST", InterruptPath, strings.NewReader(`{"index":2}`)))

	if rec.Code != 400 {
		t.Fatalf("POST without a sentence = %d, want 400", rec.Code)
	}
}
