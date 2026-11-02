// Package session is the session store: conversations saved to disk as plain
// JSON, one file per session.
//
// Files over a database, on purpose. A session you can open in a text editor
// is a session you can read, diff, back up, and delete by hand. The harness is
// fast enough that reading a file per session is not a cost worth hiding
// behind a database.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/usually-broken/lola/core/home"
	"github.com/usually-broken/lola/core/llm"
)

// DefaultDir is the folder sessions live in, beside lola.yaml.
const DefaultDir = "sessions"

// Session is one saved conversation.
type Session struct {
	// ID identifies the session. Unique within a store.
	ID string `json:"id"`

	// Title is a short human label, shown in a session list.
	Title string `json:"title"`

	// Created and Updated are timestamps, RFC 3339.
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`

	// Pinned sessions sort to the top and are not auto-pruned.
	Pinned bool `json:"pinned"`

	// Archived sessions are kept but hidden from the default list.
	Archived bool `json:"archived"`

	// Messages is the whole conversation.
	Messages []llm.Message `json:"messages"`
}

// Store is a folder of session files.
type Store struct {
	dir string
}

// NewStore returns a store rooted at dir. The directory is created on first
// save, not now, so a store can be constructed before anything is written.
// An empty dir means the Lola home directory's sessions folder.
func NewStore(dir string) *Store {
	if dir == "" {
		dir = home.SessionsDir()
	}
	return &Store{dir: dir}
}

// Dir returns the folder the store reads and writes.
func (s *Store) Dir() string { return s.dir }

// Save writes a session to disk, creating the folder if needed. It sets the
// Updated time. A session with no ID gets one.
func (s *Store) Save(sess *Session) error {
	if sess == nil {
		return fmt.Errorf("session: cannot save nil")
	}
	if sess.ID == "" {
		id, err := NewID()
		if err != nil {
			return err
		}
		sess.ID = id
	}
	if sess.Created.IsZero() {
		sess.Created = time.Now()
	}
	sess.Updated = time.Now()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("session: create store directory: %w", err)
	}

	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return fmt.Errorf("session: encode session: %w", err)
	}

	path := s.path(sess.ID)
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("session: write %s: %w", path, err)
	}
	return nil
}

// Load reads one session by ID.
func (s *Store) Load(id string) (*Session, error) {
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return nil, fmt.Errorf("session: read %s: %w", id, err)
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("session: decode %s: %w", id, err)
	}
	return &sess, nil
}

// List returns every session, newest first, with pinned sessions on top. The
// session bodies are loaded, so callers can show message counts and titles
// without a second pass.
func (s *Store) List() ([]*Session, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // an empty store is not an error
		}
		return nil, fmt.Errorf("session: read store directory: %w", err)
	}

	var sessions []*Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		sess, err := s.Load(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			// Skip a file we cannot read rather than failing the whole list:
			// one bad session should not hide the rest.
			continue
		}
		sessions = append(sessions, sess)
	}

	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i].Pinned != sessions[j].Pinned {
			return sessions[i].Pinned
		}
		if !sessions[i].Updated.Equal(sessions[j].Updated) {
			return sessions[i].Updated.After(sessions[j].Updated)
		}
		// Two sessions saved inside the same clock tick have no order in time at
		// all, which happens on platforms with a coarse timer. Falling back to
		// something stable keeps the list from shuffling between refreshes.
		if !sessions[i].Created.Equal(sessions[j].Created) {
			return sessions[i].Created.After(sessions[j].Created)
		}
		return sessions[i].ID < sessions[j].ID
	})
	return sessions, nil
}

// Delete removes a session's file.
func (s *Store) Delete(id string) error {
	err := os.Remove(s.path(id))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("session: delete %s: %w", id, err)
	}
	return nil
}

// SetPinned marks a session pinned or not, and saves it.
func (s *Store) SetPinned(id string, pinned bool) error {
	sess, err := s.Load(id)
	if err != nil {
		return err
	}
	sess.Pinned = pinned
	return s.Save(sess)
}

// SetArchived marks a session archived or not, and saves it.
func (s *Store) SetArchived(id string, archived bool) error {
	sess, err := s.Load(id)
	if err != nil {
		return err
	}
	sess.Archived = archived
	return s.Save(sess)
}

// path is where a session's file lives.
func (s *Store) path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

// NewID returns a short random session id.
func NewID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session: generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
