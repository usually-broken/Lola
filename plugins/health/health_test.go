package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/routes"
)

// tree is the smallest thing that can stand in for the plugin tree: it answers
// what is mounted and refuses the rest, which is all a health check asks.
type tree struct{ mounted []string }

func (t *tree) Mounted() []string   { return t.mounted }
func (t *tree) Available() []string { return nil }
func (t *tree) Get(string) (core.Plugin, bool) {
	return nil, false
}
func (t *tree) Mount(string) error   { return nil }
func (t *tree) Unmount(string) error { return nil }

// The plugin offers its path when something can serve it, and the path answers
// with what the tree says.
func TestThePluginOffersItsPath(t *testing.T) {
	reg := routes.New()

	p, err := plugin.Build("health", plugin.Deps{
		Routes:  reg,
		Plugins: &tree{mounted: []string{"standard", "health"}},
	})
	if err != nil {
		t.Fatalf("could not build the plugin: %v", err)
	}
	if _, err := p.Setup(); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	h, ok := reg.Match(Path)
	if !ok {
		t.Fatalf("the plugin did not offer %s", Path)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Path, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		OK      bool     `json:"ok"`
		Mounted []string `json:"mounted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer was not JSON: %v (%q)", err, rec.Body.String())
	}
	if !body.OK {
		t.Error("the answer did not say it was ok")
	}
	if len(body.Mounted) != 2 {
		t.Errorf("mounted = %v, want the tree it was given", body.Mounted)
	}
}

// A tree where nothing can answer HTTP is not an error: there is simply no path
// to offer, and the plugin mounts and does nothing.
func TestNothingToOfferIsNotAnError(t *testing.T) {
	p, err := plugin.Build("health", plugin.Deps{})
	if err != nil {
		t.Fatalf("could not build the plugin: %v", err)
	}
	if _, err := p.Setup(); err != nil {
		t.Fatalf("a tree with no HTTP frontend was treated as a failure: %v", err)
	}
}

// It answers a read, and refuses anything that is not one.
func TestItAnswersAReadAndRefusesTheRest(t *testing.T) {
	reg := routes.New()
	p, err := plugin.Build("health", plugin.Deps{Routes: reg})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Setup(); err != nil {
		t.Fatal(err)
	}

	h, ok := reg.Match(Path)
	if !ok {
		t.Fatal("the plugin did not offer its path")
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, Path, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("a write was answered with %d, want 405", rec.Code)
	}
}

// page is the smallest thing that can stand in for an interface collecting what
// plugins offer it.
type page struct{ items []plugin.Contribution }

func (p *page) Add(c plugin.Contribution)  { p.items = append(p.items, c) }
func (p *page) All() []plugin.Contribution { return p.items }

// The plugin draws a line for the interface, from the tree it was given, and
// offers it where the interface keeps what plugins have to say.
func TestThePluginDrawsALineForTheInterface(t *testing.T) {
	out := &page{}

	p, err := plugin.Build("health", plugin.Deps{
		Plugins:       &tree{mounted: []string{"standard", "health"}},
		Contributions: out,
	})
	if err != nil {
		t.Fatalf("could not build the plugin: %v", err)
	}
	if _, err := p.Setup(); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if len(out.items) != 1 {
		t.Fatalf("offered %d contributions, want 1", len(out.items))
	}
	c := out.items[0]
	if c.Place != PlaceInPage {
		t.Errorf("drawn in %q, want %q", c.Place, PlaceInPage)
	}
	if c.Kind != plugin.KindFragment {
		t.Errorf("offered a %q, want a fragment", c.Kind)
	}
	if c.Draw == nil {
		t.Fatal("there is no markup to draw")
	}
	if !strings.Contains(c.Draw(), "2 mounted") {
		t.Errorf("the line says %q, want the count it was given", c.Draw())
	}
}

// A tree with nowhere to draw is not an error either: the line is offered where
// it can be, and does not bring the plugin down where it cannot.
func TestNowhereToDrawIsNotAnError(t *testing.T) {
	p, err := plugin.Build("health", plugin.Deps{})
	if err != nil {
		t.Fatalf("could not build the plugin: %v", err)
	}
	if _, err := p.Setup(); err != nil {
		t.Fatalf("a tree with no interface was treated as a failure: %v", err)
	}
}
