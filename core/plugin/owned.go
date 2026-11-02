package plugin

// owned is a plugin's view of the contributions seam.
//
// It is the same list, with the offering plugin's name stamped on everything
// that goes through it. The stamp comes from the harness rather than from the
// plugin, so nothing can claim to be from somewhere it isn't, and a plugin that
// never said who it was is still identifiable when something needs tracking
// down.
//
// Read is passed straight through: a plugin offering something has no business
// seeing only its own offering, and asking for the list has nothing to do with
// who is asking.
type owned struct {
	name string
	next Contributions
}

// Add stamps the contribution and passes it on.
func (o *owned) Add(c Contribution) {
	c.Owner = o.name
	o.next.Add(c)
}

// All passes the read through unchanged.
func (o *owned) All() []Contribution { return o.next.All() }

// Assert at compile time that this satisfies the seam.
var _ Contributions = (*owned)(nil)
