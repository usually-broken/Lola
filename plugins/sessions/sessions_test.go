package sessions

import (
	"path/filepath"
	"testing"

	"github.com/usually-broken/lola/core/home"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/session"
)

// The whole of what this plugin contributes is being a keeper. If that assertion
// stops holding, the loop and the frontends quietly go back to building their
// own stores and nobody notices until sessions disagree.
func TestThePluginIsAKeeper(t *testing.T) {
	var p any = &Plugin{store: session.NewStore(filepath.Join(t.TempDir(), session.DefaultFile))}
	if _, ok := p.(session.Keeper); !ok {
		t.Fatal("the sessions plugin is not a session.Keeper, so nothing can find it")
	}
	if p.(session.Keeper).Sessions() == nil {
		t.Fatal("the keeper handed back nothing")
	}
}

// It is registered under the name a config uses, and it is brought in by the
// catalog, or no config could mount it.
func TestItIsRegistered(t *testing.T) {
	for _, name := range plugin.Names() {
		if name == "sessions" {
			return
		}
	}
	t.Fatalf("sessions is not registered; the catalog has %v", plugin.Names())
}

// The store it hands out is the one over the harness's sessions folder, so what
// the loop writes is what the command line reads back rather than two folders
// that happen to look alike.
func TestTheStoreIsOverTheHomesSessionsFolder(t *testing.T) {
	p := &Plugin{store: session.NewStore(filepath.Join(home.SessionsDir(), session.DefaultFile))}
	want := filepath.Join(home.SessionsDir(), session.DefaultFile)
	if got := p.Sessions().Path(); got != want {
		t.Fatalf("the store is over %q, want %q", got, want)
	}
}
