package memory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usually-broken/lola/core/prompt"
	"github.com/usually-broken/lola/core/tools"
)

func newPlugin(t *testing.T) (*Plugin, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.md")
	p := New(tools.New(), prompt.New(), path)
	if _, err := p.Setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return p, path
}

func run(t *testing.T, p *Plugin, in string) string {
	t.Helper()
	out, err := p.Run(context.Background(), json.RawMessage(in))
	if err != nil {
		t.Fatalf("run %s: %v", in, err)
	}
	return out
}

func TestAddKeepsEntriesApart(t *testing.T) {
	p, path := newPlugin(t)

	run(t, p, `{"action":"add","text":"first thing"}`)
	run(t, p, `{"action":"add","text":"second thing"}`)

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := strings.Count(string(body), Separator); got != 1 {
		t.Fatalf("separator count = %d, want 1:\n%s", got, body)
	}
	if !strings.Contains(string(body), "first thing") || !strings.Contains(string(body), "second thing") {
		t.Fatalf("an entry went missing:\n%s", body)
	}
}

func TestPromptTextIsWhatIsOnDisk(t *testing.T) {
	p, _ := newPlugin(t)

	run(t, p, `{"action":"add","text":"likes Go"}`)
	if got := p.text(); !strings.Contains(got, "likes Go") {
		t.Fatalf("prompt text = %q", got)
	}

	run(t, p, `{"action":"replace","index":1,"new":"likes Go a lot"}`)
	got := p.text()
	if strings.Contains(got, "likes Go\n") || !strings.Contains(got, "likes Go a lot") {
		t.Fatalf("replacement did not take: %q", got)
	}
}

func TestReplaceByPhrase(t *testing.T) {
	p, _ := newPlugin(t)

	run(t, p, `{"action":"add","text":"tally is on 6"}`)
	run(t, p, `{"action":"add","text":"puppy arrives Saturday"}`)

	run(t, p, `{"action":"replace","old":"puppy","new":"puppy arrives on the fourteenth"}`)

	entries := p.entries()
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if !strings.Contains(entries[1], "fourteenth") {
		t.Fatalf("wrong entry replaced: %#v", entries)
	}
	if !strings.Contains(entries[0], "tally") {
		t.Fatalf("first entry was disturbed: %#v", entries)
	}
}

func TestReadIsNumbered(t *testing.T) {
	p, _ := newPlugin(t)

	run(t, p, `{"action":"add","text":"one"}`)
	run(t, p, `{"action":"add","text":"two"}`)

	out := run(t, p, `{"action":"read"}`)
	if !strings.HasPrefix(out, "1. one") || !strings.Contains(out, "2. two") {
		t.Fatalf("read output = %q", out)
	}
}

func TestEmptyMemoryHasNoPromptPart(t *testing.T) {
	p, _ := newPlugin(t)
	if got := p.text(); got != "" {
		t.Fatalf("empty memory contributed %q", got)
	}
}

func TestAddRefusesNothing(t *testing.T) {
	p, _ := newPlugin(t)
	if _, err := p.Run(context.Background(), json.RawMessage(`{"action":"add","text":"  "}`)); err == nil {
		t.Fatal("expected an error for an empty entry")
	}
}

func TestDeleteRemovesTheEntry(t *testing.T) {
	p, _ := newPlugin(t)

	run(t, p, `{"action":"add","text":"keep me"}`)
	run(t, p, `{"action":"add","text":"bin me"}`)
	run(t, p, `{"action":"replace","index":2,"delete":true}`)

	left := p.entries()
	if len(left) != 1 || !strings.Contains(left[0], "keep me") {
		t.Fatalf("delete left %#v", left)
	}
	if strings.Contains(p.text(), "bin me") {
		t.Fatal("deleted entry still in the prompt text")
	}
}

func TestTeardownTakesThePromptPartWithIt(t *testing.T) {
	prompts := prompt.New()
	p := New(tools.New(), prompts, filepath.Join(t.TempDir(), "memory.md"))

	teardown, err := p.Setup()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	run(t, p, `{"action":"add","text":"blue is the favourite colour"}`)
	if !strings.Contains(prompts.Text(), "blue") {
		t.Fatalf("prompt part missing: %q", prompts.Text())
	}

	if err := teardown(); err != nil {
		t.Fatalf("teardown: %v", err)
	}
	if strings.Contains(prompts.Text(), "blue") {
		t.Fatalf("memory still in the prompt after unmounting: %q", prompts.Text())
	}
}
