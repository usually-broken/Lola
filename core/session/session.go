// Package session is the session store: conversations saved in one SQLite
// database.
//
// A database over files, on purpose. One session is a row, its messages are
// rows of their own, and the whole history can be searched without opening
// every conversation. The store keeps the same surface it had when sessions
// were JSON files, so the agent loop, the web pane and the command line need
// no changes to use it.
package session

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver: no cgo, so the harness still cross-builds

	"github.com/usually-broken/lola/core/home"
	"github.com/usually-broken/lola/core/llm"
)

// DefaultFile is the database, beside lola.yaml.
const DefaultFile = "sessions.db"

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

// Store is one database of sessions.
type Store struct {
	path string
	db   *sql.DB
}

// NewStore returns a store backed by the database at path. The file and its
// schema are created on first use. An empty path means the Lola home
// directory's sessions database.
func NewStore(path string) *Store {
	if path == "" {
		path = filepath.Join(home.SessionsDir(), DefaultFile)
	}
	return &Store{path: path}
}

// Path returns the database file the store reads and writes.
func (s *Store) Path() string { return s.path }

// Close releases the database handle.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// open connects and creates the schema if it is not there yet.
func (s *Store) open() (*sql.DB, error) {
	if s.db != nil {
		return s.db, nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return nil, fmt.Errorf("session: create store directory: %w", err)
	}
	db, err := sql.Open("sqlite", s.path)
	if err != nil {
		return nil, fmt.Errorf("session: open %s: %w", s.path, err)
	}
	// One writer at a time: the web pane and the agent loop share this handle.
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id        TEXT PRIMARY KEY,
			title     TEXT NOT NULL DEFAULT '',
			created   TEXT NOT NULL,
			updated   TEXT NOT NULL,
			pinned    INTEGER NOT NULL DEFAULT 0,
			archived  INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			position   INTEGER NOT NULL,
			role       TEXT NOT NULL,
			body       TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS messages_by_session ON messages(session_id, position)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("session: prepare database: %w", err)
		}
	}
	s.db = db
	return db, nil
}

// Save writes a session, creating it if it is new. It sets the Updated time.
// A session with no ID gets one. Messages are replaced wholesale, so the
// caller's slice is the session exactly as it now stands.
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

	db, err := s.open()
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("session: begin: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO sessions (id, title, created, updated, pinned, archived)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   title=excluded.title, updated=excluded.updated,
		   pinned=excluded.pinned, archived=excluded.archived`,
		sess.ID, sess.Title,
		sess.Created.Format(time.RFC3339Nano), sess.Updated.Format(time.RFC3339Nano),
		boolInt(sess.Pinned), boolInt(sess.Archived),
	)
	if err != nil {
		return fmt.Errorf("session: write %s: %w", sess.ID, err)
	}

	if _, err := tx.Exec(`DELETE FROM messages WHERE session_id = ?`, sess.ID); err != nil {
		return fmt.Errorf("session: clear messages: %w", err)
	}
	for i, m := range sess.Messages {
		body, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("session: encode message: %w", err)
		}
		if _, err := tx.Exec(
			`INSERT INTO messages (session_id, position, role, body) VALUES (?, ?, ?, ?)`,
			sess.ID, i, m.Role, string(body),
		); err != nil {
			return fmt.Errorf("session: write message: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("session: commit %s: %w", sess.ID, err)
	}
	return nil
}

// Load reads one session by ID.
func (s *Store) Load(id string) (*Session, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	sess := &Session{}
	var created, updated string
	var pinned, archived int
	err = db.QueryRow(
		`SELECT id, title, created, updated, pinned, archived FROM sessions WHERE id = ?`, id,
	).Scan(&sess.ID, &sess.Title, &created, &updated, &pinned, &archived)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session: no session %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("session: read %s: %w", id, err)
	}
	sess.Created, _ = time.Parse(time.RFC3339Nano, created)
	sess.Updated, _ = time.Parse(time.RFC3339Nano, updated)
	sess.Pinned = pinned != 0
	sess.Archived = archived != 0

	rows, err := db.Query(
		`SELECT body FROM messages WHERE session_id = ? ORDER BY position`, id)
	if err != nil {
		return nil, fmt.Errorf("session: read messages for %s: %w", id, err)
	}
	defer rows.Close()
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, fmt.Errorf("session: scan message: %w", err)
		}
		var m llm.Message
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			continue // one unreadable message should not hide the rest
		}
		sess.Messages = append(sess.Messages, m)
	}
	return sess, rows.Err()
}

// List returns every session, newest first, with pinned sessions on top. The
// session bodies are loaded, so callers can show message counts and titles
// without a second pass.
func (s *Store) List() ([]*Session, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id FROM sessions`)
	if err != nil {
		return nil, fmt.Errorf("session: list: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("session: scan id: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("session: list: %w", err)
	}

	var sessions []*Session
	for _, id := range ids {
		sess, err := s.Load(id)
		if err != nil {
			continue // skip a session we cannot read rather than failing the list
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
		// all. Falling back to something stable keeps the list from shuffling.
		if !sessions[i].Created.Equal(sessions[j].Created) {
			return sessions[i].Created.After(sessions[j].Created)
		}
		return sessions[i].ID < sessions[j].ID
	})
	return sessions, nil
}

// ListFull is List with the messages left on: what a caller needs when it is
// reading replies rather than drawing a list.
func (s *Store) ListFull() ([]*Session, error) { return s.List() }

// Delete removes a session and its messages.
func (s *Store) Delete(id string) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM messages WHERE session_id = ?`, id); err != nil {
		return fmt.Errorf("session: delete messages for %s: %w", id, err)
	}
	if _, err := db.Exec(`DELETE FROM sessions WHERE id = ?`, id); err != nil {
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

// Search finds sessions whose messages contain every word of query, newest
// first. This is the reason the store is a database: the whole history is
// askable without loading it.
func (s *Store) Search(query string, limit int) ([]*Session, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.Query(
		`SELECT DISTINCT session_id FROM messages WHERE body LIKE ? LIMIT ?`,
		"%"+query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("session: search: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("session: scan search hit: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("session: search: %w", err)
	}

	var found []*Session
	for _, id := range ids {
		if sess, err := s.Load(id); err == nil {
			found = append(found, sess)
		}
	}
	return found, nil
}

// boolInt turns a bool into the 0/1 SQLite stores.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// NewID returns a short random session id.
func NewID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session: generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
