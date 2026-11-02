package guard

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// rules builds a policy for a temporary home, in the given mode.
func rules(t *testing.T, mode Mode) (*Rules, string) {
	t.Helper()
	home := t.TempDir()
	r := Default(home, "")
	if err := r.SetMode(mode); err != nil {
		t.Fatalf("setting the mode: %v", err)
	}
	return r, home
}

// callArgs is a raw JSON object, the way a tool call arrives.
func callArgs(t *testing.T, kv map[string]string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(kv)
	if err != nil {
		t.Fatalf("building arguments: %v", err)
	}
	return b
}

// Ask mode puts a tool the harness has never heard of to a person. This is the
// whole point of the mode: a tree full of plugins has tools no list here could
// name, and anything unnamed used to be allowed without a word.
func TestAskCoversAToolNobodyHasHeardOf(t *testing.T) {
	r, _ := rules(t, ModeAsk)

	if got := r.Check("word_count", callArgs(t, map[string]string{"text": "hello"})); got != Ask {
		t.Errorf("a plugin's tool decided %s in ask mode, want ask", got)
	}
	if got := r.Check("send_email", callArgs(t, map[string]string{"to": "someone"})); got != Ask {
		t.Errorf("another plugin's tool decided %s in ask mode, want ask", got)
	}
}

// Ask mode puts reading to a person too. It cannot be picky: it has no way to
// know what an unnamed tool will do with the file it reads, and a mode that
// guesses is a mode that is wrong about the one tool that mattered.
func TestAskCoversReadingToo(t *testing.T) {
	r, _ := rules(t, ModeAsk)

	if got := r.Check("read_file", callArgs(t, map[string]string{"path": "/tmp/notes.txt"})); got != Ask {
		t.Errorf("reading decided %s in ask mode, want ask", got)
	}
	if got := r.Check("run_shell", callArgs(t, map[string]string{"command": "ls -la"})); got != Ask {
		t.Errorf("an ordinary command decided %s in ask mode, want ask", got)
	}
}

// A protected path is refused outright, not offered. Asking would make it a
// preference, and the only thing that makes it safe to hand the harness more
// rope is that some doors do not open whatever anybody says.
func TestAProtectedPathIsRefusedBeforeItIsOffered(t *testing.T) {
	r, home := rules(t, ModeAsk)
	protected := filepath.Join(home, "sessions", "abc.json")

	if got := r.Check("read_file", callArgs(t, map[string]string{"path": protected})); got != Deny {
		t.Errorf("a protected path decided %s in ask mode, want deny", got)
	}
	// Including for a tool that is not one of ours: it reaches the same files.
	if got := r.Check("cat_file", callArgs(t, map[string]string{"path": protected})); got != Deny {
		t.Errorf("a plugin's tool on a protected path decided %s, want deny", got)
	}
}

// Open mode asks nothing, and still refuses the protected paths.
func TestOpenAsksNothingButStillRefusesTheWall(t *testing.T) {
	r, home := rules(t, ModeOpen)
	protected := filepath.Join(home, "profiles", "web.json")

	if got := r.Check("word_count", callArgs(t, map[string]string{"text": "hi"})); got != Allow {
		t.Errorf("a plugin's tool decided %s in open mode, want allow", got)
	}
	if got := r.Check("read_file", callArgs(t, map[string]string{"path": protected})); got != Deny {
		t.Errorf("a protected path decided %s in open mode, want deny", got)
	}
}

// Workspace mode is unchanged for the tools it knows: reading is free, and a
// write inside the workspace runs without being put to anybody.
func TestWorkspaceModeKeepsItsOldBehaviour(t *testing.T) {
	r, _ := rules(t, ModeWorkspace)

	if got := r.Check("read_file", callArgs(t, map[string]string{"path": "/tmp/notes.txt"})); got != Allow {
		t.Errorf("reading decided %s in workspace mode, want allow", got)
	}
	if got := r.Check("write_file", callArgs(t, map[string]string{"path": "/tmp/notes.txt"})); got != Allow {
		t.Errorf("an ordinary write decided %s in workspace mode, want allow", got)
	}
	if got := r.Check("run_shell", callArgs(t, map[string]string{"command": "rm -rf /tmp/thing"})); got != Ask {
		t.Errorf("a dangerous command decided %s in workspace mode, want ask", got)
	}
}

// The remaining gap, written down so it is a decision rather than an oversight:
// in workspace mode a tool nobody has heard of is still allowed. Ask mode is
// the mode for a tree you did not write; making workspace mode suspicious of
// every unnamed tool is a separate change, and this test is the note that it
// has not been made.
func TestWorkspaceModeStillTrustsToolsItDoesNotKnow(t *testing.T) {
	r, _ := rules(t, ModeWorkspace)

	if got := r.Check("word_count", callArgs(t, map[string]string{"text": "hi"})); got != Allow {
		t.Errorf("a plugin's tool decided %s in workspace mode, want allow (for now)", got)
	}
}
