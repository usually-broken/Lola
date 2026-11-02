package speak

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The mark is the whole of this change: what is being said, where in the reply, and
// how far in. Nothing reads it yet, so the test is about honesty of the record.
func TestSpokenMarkRoundTrips(t *testing.T) {
	p := &Plugin{}

	body := `{"speaking":true,"sentence":"The thing is, I was going to say","index":2,"of":5,"offsetMs":800}`
	req := httptest.NewRequest(http.MethodPost, SpokenPath, strings.NewReader(body))
	rec := httptest.NewRecorder()
	p.HandleSpoken(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST = %d, want 204", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, SpokenPath, nil)
	rec = httptest.NewRecorder()
	p.HandleSpoken(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d, want 200", rec.Code)
	}

	var got Spoken
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if !got.Speaking || got.Index != 2 || got.Of != 5 || got.OffsetMS != 800 {
		t.Fatalf("mark = %+v", got)
	}
	if got.Sentence != "The thing is, I was going to say" {
		t.Fatalf("sentence = %q", got.Sentence)
	}
	if got.StartedAt == "" {
		t.Fatal("no start time recorded: a mark without a time cannot be resumed from")
	}
}

// Finishing clears the mark rather than leaving a sentence nobody is saying any more.
func TestSpokenMarkClears(t *testing.T) {
	p := &Plugin{}
	p.HandleSpoken(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, SpokenPath,
		strings.NewReader(`{"speaking":true,"sentence":"Half a thought","index":1}`)))

	rec := httptest.NewRecorder()
	p.HandleSpoken(rec, httptest.NewRequest(http.MethodPost, SpokenPath,
		strings.NewReader(`{"speaking":false}`)))

	if p.SpokenNow().Speaking {
		t.Fatal("still marked as speaking after being told it stopped")
	}
	if p.SpokenNow().Sentence != "" {
		t.Fatal("a finished sentence is still on the record")
	}
}

// A body that is not a mark is refused, because silence about a bad request is how a
// bug survives a test suite.
func TestSpokenRefusesRubbish(t *testing.T) {
	p := &Plugin{}
	rec := httptest.NewRecorder()
	p.HandleSpoken(rec, httptest.NewRequest(http.MethodPost, SpokenPath, strings.NewReader(`not json`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST rubbish = %d, want 400", rec.Code)
	}
}
