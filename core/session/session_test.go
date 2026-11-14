package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/usually-broken/lola/core/llm"
)

func tmpStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(filepath.Join(t.TempDir(), "sessions", "sessions.db"))
}

func msg(role llm.Role, content string) llm.Message {
	return llm.Message{Role: role, Content: content}
}

func TestSaveThenLoad(t *testing.T) {
	s := tmpStore(t)
	sess := &Session{
		Title:    "a chat",
		Messages: []llm.Message{msg(llm.RoleUser, "hello")},
	}
	if err := s.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("Save should assign an id")
	}

	got, err := s.Load(sess.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Title != "a chat" || len(got.Messages) != 1 || got.Messages[0].Content != "hello" {
		t.Fatalf("loaded session = %+v", got)
	}
}

func TestSaveWritesOneDatabaseFile(t *testing.T) {
	s := tmpStore(t)
	sess := &Session{Title: "readable", Messages: []llm.Message{msg(llm.RoleUser, "hi")}}
	if err := s.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Every session lives in the one database, not in a file each.
	if _, err := os.Stat(s.Path()); err != nil {
		t.Fatalf("database file: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(s.Path()))
	if err != nil {
		t.Fatalf("list store: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			t.Fatalf("session written as JSON beside the database: %s", e.Name())
		}
	}
}

func TestListEmptyStoreIsNotAnError(t *testing.T) {
	s := tmpStore(t) // directory does not exist yet
	list, err := s.List()
	if err != nil {
		t.Fatalf("List on a fresh store: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("list = %v, want empty", list)
	}
}

func TestListSortsPinnedFirstThenNewest(t *testing.T) {
	s := tmpStore(t)

	first := &Session{Title: "first"}
	if err := s.Save(first); err != nil {
		t.Fatal(err)
	}
	// Long enough that the two saves cannot land in the same clock tick. On a
	// coarse timer they would be equally new, and the order of two things saved
	// at the same instant is genuinely undefined — not something to assert on.
	time.Sleep(25 * time.Millisecond)
	second := &Session{Title: "second"}
	if err := s.Save(second); err != nil {
		t.Fatal(err)
	}
	pinned := &Session{Title: "pinned", Pinned: true}
	if err := s.Save(pinned); err != nil {
		t.Fatal(err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("list has %d sessions, want 3", len(list))
	}
	if list[0].Title != "pinned" {
		t.Fatalf("first should be the pinned session, got %q", list[0].Title)
	}
	// Newest first after the pinned one.
	if list[1].Title != "second" {
		t.Fatalf("second should be the newest unpinned, got %q", list[1].Title)
	}
}

func TestSetPinnedAndArchived(t *testing.T) {
	s := tmpStore(t)
	sess := &Session{Title: "x"}
	if err := s.Save(sess); err != nil {
		t.Fatal(err)
	}

	if err := s.SetPinned(sess.ID, true); err != nil {
		t.Fatalf("SetPinned: %v", err)
	}
	if err := s.SetArchived(sess.ID, true); err != nil {
		t.Fatalf("SetArchived: %v", err)
	}

	got, err := s.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Pinned || !got.Archived {
		t.Fatalf("flags not persisted: %+v", got)
	}
}

func TestDeleteRemovesFile(t *testing.T) {
	s := tmpStore(t)
	sess := &Session{Title: "gone"}
	if err := s.Save(sess); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Load(sess.ID); err == nil {
		t.Fatal("session should be gone after Delete")
	}
}

func TestDeleteMissingIsNotAnError(t *testing.T) {
	s := tmpStore(t)
	if err := s.Delete("nope"); err != nil {
		t.Fatalf("deleting a missing session should not error: %v", err)
	}
}

func TestListSkipsUnreadableFiles(t *testing.T) {
	s := tmpStore(t)
	good := &Session{Title: "good"}
	if err := s.Save(good); err != nil {
		t.Fatal(err)
	}
	// A junk message row must not hide the real sessions.
	db, err := s.open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO messages (session_id, position, role, body) VALUES ('good', 9, 'user', '{not json')`); err != nil {
		t.Fatal(err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Title != "good" {
		t.Fatalf("list = %+v, want just the good session", list)
	}
}

func TestNewIDIsRandom(t *testing.T) {
	a, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf("two ids collided: %q", a)
	}
	if len(a) != 8 {
		t.Fatalf("id %q should be 8 hex chars", a)
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, n := range needles {
		found := false
		for i := 0; i+len(n) <= len(haystack); i++ {
			if haystack[i:i+len(n)] == n {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
