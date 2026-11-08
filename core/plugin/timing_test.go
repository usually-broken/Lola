package plugin

import (
	"testing"
	"time"
)

// The maths is the whole seam, so it is tested on known numbers rather than on a
// live provider.
func TestTurnMeasurements(t *testing.T) {
	start := time.Date(2026, 11, 5, 19, 0, 0, 0, time.UTC)
	turn := Turn{
		Started: start,
		First:   start.Add(400 * time.Millisecond),
		Done:    start.Add(1400 * time.Millisecond),
		Chars:   100,
	}

	wait, ok := turn.FirstWait()
	if !ok || wait != 400*time.Millisecond {
		t.Fatalf("first wait = %v, %v", wait, ok)
	}

	// 100 characters in the second between the first word and the last.
	rate, ok := turn.CharsPerSecond()
	if !ok {
		t.Fatal("no rate")
	}
	if rate < 99 || rate > 101 {
		t.Fatalf("rate = %.2f, want about 100", rate)
	}
}

// A provider that does not stream has no first word, and says so rather than
// dividing by nothing.
func TestUnstreamedTurnHasNoFirstWord(t *testing.T) {
	start := time.Date(2026, 11, 5, 19, 0, 0, 0, time.UTC)
	turn := Turn{Started: start, Done: start.Add(time.Second), Chars: 50}

	if _, ok := turn.FirstWait(); ok {
		t.Fatal("claimed a first word that never arrived")
	}
	if _, ok := turn.CharsPerSecond(); ok {
		t.Fatal("claimed a rate with nothing to divide by")
	}
}

// Listening is what the plugins do, and a report reaches every listener.
func TestReportReachesEveryListener(t *testing.T) {
	var first, second int
	OnTurn(func(Turn) { first++ })
	OnTurn(func(Turn) { second++ })

	ReportTurn(Turn{Started: time.Now()})

	if first == 0 || second == 0 {
		t.Fatalf("listeners saw %d and %d", first, second)
	}
}
