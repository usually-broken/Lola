package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/usually-broken/lola/core/plugin"
)

// offered is a stand-in for the contributions seam, so the endpoint can be
// tested without mounting a plugin to fill it.
type offered struct{ items []plugin.Contribution }

func (o *offered) Add(c plugin.Contribution)  { o.items = append(o.items, c) }
func (o *offered) All() []plugin.Contribution { return o.items }

// What a plugin offered is what the page is told, with the place and the kind
// intact, because the page decides both how to draw it and where.
func TestThePageIsToldWhatPluginsOffered(t *testing.T) {
	seam := &offered{}
	seam.Add(plugin.Contribution{
		Place:   "above-composer",
		Kind:    plugin.KindSelect,
		Name:    "mode",
		Label:   "Mode",
		Value:   "standard",
		Options: []string{"standard", "creator"},
		Set:     func(value string) error { return nil },
	})

	u := &ui{contributions: seam}
	rec := httptest.NewRecorder()
	u.contributionsGet(rec, httptest.NewRequest(http.MethodGet, "/api/contributions", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}

	var got []plugin.Contribution
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("the reply was not readable: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d contributions, want 1", len(got))
	}
	if got[0].Place != "above-composer" || got[0].Kind != plugin.KindSelect {
		t.Fatalf("got place %q kind %q", got[0].Place, got[0].Kind)
	}
	if got[0].Value != "standard" || len(got[0].Options) != 2 {
		t.Fatalf("got value %q with %d options", got[0].Value, len(got[0].Options))
	}
}

// With nothing collecting contributions the page gets an empty list rather
// than an error, because nothing offering anything is an ordinary state.
func TestThePageGetsAnEmptyListWithNothingOffered(t *testing.T) {
	u := &ui{}
	rec := httptest.NewRecorder()
	u.contributionsGet(rec, httptest.NewRequest(http.MethodGet, "/api/contributions", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "[]" {
		t.Fatalf("got %q, want []", body)
	}
}

// A contribution asking for a place this interface does not have is not handed
// to the page: the page would have nowhere to draw it and no way to say so.
func TestAContributionForAPlaceWeDoNotHaveIsDropped(t *testing.T) {
	seam := &offered{}
	seam.Add(plugin.Contribution{Place: "above-composer", Kind: plugin.KindChip, Name: "fine", Label: "Fine"})
	seam.Add(plugin.Contribution{Place: "nowhere-at-all", Kind: plugin.KindChip, Name: "lost", Label: "Lost"})

	u := &ui{contributions: seam}
	rec := httptest.NewRecorder()
	u.contributionsGet(rec, httptest.NewRequest(http.MethodGet, "/api/contributions", nil))

	var got []plugin.Contribution
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("the reply was not readable: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d contributions, want only the one we can place", len(got))
	}
	if got[0].Name != "fine" {
		t.Fatalf("got %q, want fine", got[0].Name)
	}
}

// The drop is said out loud, naming the plugin and the place, so a control that
// never appears is a line of output rather than a mystery. It is said once, not
// on every poll.
func TestAMissingPlaceIsSaidOutLoudOnce(t *testing.T) {
	var said bytes.Buffer
	log.SetOutput(&said)
	defer log.SetOutput(os.Stderr)

	seam := &offered{}
	seam.Add(plugin.Contribution{
		Owner: "modechip",
		Place: "nowhere-at-all",
		Kind:  plugin.KindChip,
		Name:  "lost",
		Label: "Lost",
	})

	u := &ui{contributions: seam}
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		u.contributionsGet(rec, httptest.NewRequest(http.MethodGet, "/api/contributions", nil))
	}

	out := said.String()
	if !strings.Contains(out, "modechip") {
		t.Errorf("the output does not name the plugin that asked: %q", out)
	}
	if !strings.Contains(out, "nowhere-at-all") {
		t.Errorf("the output does not name the place that was asked for: %q", out)
	}
	if n := strings.Count(out, "nowhere-at-all"); n != 1 {
		t.Fatalf("said it %d times across three reads, want once", n)
	}
}

// Every place this interface claims to have must have somewhere to go. A place
// either has a marker in the page, or it arrives inside a fragment drawn into a
// place the page does have — the composer's own controls now travel with the
// box rather than sitting in the page.
func TestEveryPlaceHasSomewhereInThePage(t *testing.T) {
	page, err := assets.ReadFile("index.html")
	if err != nil {
		t.Fatalf("could not read the page: %v", err)
	}

	insideFragments := map[string]string{
		"composer-tools": "composer",
	}

	for _, place := range places {
		marker := `data-place="` + place + `"`
		if strings.Contains(string(page), marker) {
			continue
		}
		if host, ok := insideFragments[place]; ok &&
			strings.Contains(string(page), `data-place="`+host+`"`) {
			continue
		}
		t.Errorf("the interface claims place %q but nothing in the page can hold it", place)
	}
}

// A change goes through the seam: the page names the contribution, the plugin
// that offered it applies the change. Nothing in the page knows what a mode is,
// or where a mode is set.
func TestAChangeGoesThroughTheSeam(t *testing.T) {
	applied := ""

	seam := &offered{}
	seam.Add(plugin.Contribution{
		Place: "above-composer",
		Kind:  plugin.KindSelect,
		Name:  "mode",
		Label: "Mode",
		Set:   func(value string) error { applied = value; return nil },
	})

	u := &ui{contributions: seam}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/contribution/set",
		strings.NewReader(`{"name":"mode","value":"creator"}`))
	u.contributionSet(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("got status %d, want 202: %s", rec.Code, rec.Body.String())
	}
	if applied != "creator" {
		t.Fatalf("the plugin was given %q, want creator", applied)
	}
}

// What the plugin says about a refused change reaches whoever asked. A plugin
// refusing something is an ordinary answer, not a crash.
func TestAChangeThePluginRefusesIsPassedBack(t *testing.T) {
	seam := &offered{}
	seam.Add(plugin.Contribution{
		Place: "above-composer",
		Kind:  plugin.KindSelect,
		Name:  "mode",
		Label: "Mode",
		Set:   func(value string) error { return errors.New("mode: no such mode " + value) },
	})

	u := &ui{contributions: seam}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/contribution/set",
		strings.NewReader(`{"name":"mode","value":"nonsense"}`))
	u.contributionSet(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want 500", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "no such mode") {
		t.Fatalf("the plugin's reason did not reach the caller: %q", body)
	}
}

// Something offered to be read but not touched cannot be changed, and saying so
// is better than appearing to try.
func TestAReadOnlyContributionCannotBeChanged(t *testing.T) {
	seam := &offered{}
	seam.Add(plugin.Contribution{Place: "above-composer", Kind: plugin.KindText, Name: "ttft", Label: "TTFT"})

	u := &ui{contributions: seam}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/contribution/set",
		strings.NewReader(`{"name":"ttft","value":"anything"}`))
	u.contributionSet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "cannot be changed") {
		t.Fatalf("got %q, want it to say it cannot be changed", body)
	}
}

// A name nobody offered is a bad request, because it asked for something that
// is not there.
func TestAChangeToSomethingNotOfferedIsRefused(t *testing.T) {
	u := &ui{contributions: &offered{}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/contribution/set",
		strings.NewReader(`{"name":"invented","value":"x"}`))
	u.contributionSet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "invented") {
		t.Fatalf("the refusal does not name what was asked for: %q", body)
	}
}
