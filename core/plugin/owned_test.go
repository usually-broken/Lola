package plugin

import "testing"

// collected is a stand-in for whatever is collecting contributions, so the
// wrapper can be tested without building a plugin.
type collected struct{ items []Contribution }

func (c *collected) Add(x Contribution)  { c.items = append(c.items, x) }
func (c *collected) All() []Contribution { return c.items }

// What a plugin offers comes through stamped with the plugin's name, so the
// harness never has to take a plugin's word for who it is.
func TestWhatAPluginOffersIsStampedWithItsName(t *testing.T) {
	under := &collected{}
	seam := &owned{name: "modechip", next: under}

	seam.Add(Contribution{Place: "above-composer", Kind: KindSelect, Name: "mode", Label: "Mode"})

	if len(under.items) != 1 {
		t.Fatalf("got %d contributions through, want 1", len(under.items))
	}
	if got := under.items[0].Owner; got != "modechip" {
		t.Fatalf("got owner %q, want modechip", got)
	}
}

// A plugin that tries to say who it is gets corrected. The stamp is the
// harness's, not the plugin's.
func TestAPluginCannotSayWhoItIs(t *testing.T) {
	under := &collected{}
	seam := &owned{name: "modechip", next: under}

	seam.Add(Contribution{
		Owner: "someone-else",
		Place: "above-composer",
		Kind:  KindChip,
		Name:  "imposter",
		Label: "Imposter",
	})

	if got := under.items[0].Owner; got != "modechip" {
		t.Fatalf("got owner %q, want the offering plugin, modechip", got)
	}
}

// Reading is passed straight through: a plugin offering something has no
// business seeing only its own offering.
func TestReadingIsNotFilteredByWhoIsAsking(t *testing.T) {
	under := &collected{}
	under.Add(Contribution{Place: "above-composer", Kind: KindChip, Name: "theirs", Label: "Theirs"})

	seam := &owned{name: "modechip", next: under}

	if got := seam.All(); len(got) != 1 || got[0].Name != "theirs" {
		t.Fatalf("got %d contributions, want the one already there", len(got))
	}
}
