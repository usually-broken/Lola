package speak

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// SpokenPath is where the page reports what it is saying, and where anything that
// wants to know can ask.
const SpokenPath = "/api/voice/spoken"

// Spoken is what is being said right now: which sentence, the words of it, and how
// far in. It exists so that speech can be interrupted later without losing the thread
// — you cannot pick up a half-finished thought without knowing where it stopped.
//
// Nothing reads it yet. That is deliberate: the mark comes first, and the interrupting
// is a separate change that needs it.
type Spoken struct {
	Speaking  bool   `json:"speaking"`
	Sentence  string `json:"sentence,omitempty"`
	Index     int    `json:"index,omitempty"`
	Of        int    `json:"of,omitempty"`
	OffsetMS  int    `json:"offsetMs,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
}

// spokenState is the mark plus the lock that guards it. Kept apart from Spoken so the
// mark itself stays a plain value anything may read.
type spokenState struct {
	mu   sync.RWMutex
	mark Spoken
}

// report is what the page sends: it starts a sentence, updates how far in it is, and
// says when it has finished.
type report struct {
	Speaking *bool  `json:"speaking"`
	Sentence string `json:"sentence"`
	Index    int    `json:"index"`
	Of       int    `json:"of"`
	OffsetMS int    `json:"offsetMs"`
}

// HandleSpoken answers with the current mark, or accepts a new one.
func (p *Plugin) HandleSpoken(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		p.spoken.mu.RLock()
		mark := p.spoken.mark
		p.spoken.mu.RUnlock()
		json.NewEncoder(w).Encode(mark)

	case http.MethodPost:
		var in report
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "that is not a spoken mark: "+err.Error(), http.StatusBadRequest)
			return
		}

		p.spoken.mu.Lock()
		if in.Speaking != nil && !*in.Speaking {
			// Finished or stopped: the mark clears rather than pointing at a sentence
			// nobody is saying any more. The fields are cleared one at a time: replacing
			// the struct would replace the mutex we are holding a lock on, and the
			// unlock would then fire on a lock nobody took.
			p.spoken.mark = Spoken{}
		} else {
			p.spoken.mark = Spoken{
				Speaking:  true,
				Sentence:  in.Sentence,
				Index:     in.Index,
				Of:        in.Of,
				OffsetMS:  in.OffsetMS,
				StartedAt: time.Now().UTC().Format(time.RFC3339),
			}
		}
		p.spoken.mu.Unlock()

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "GET or POST", http.StatusMethodNotAllowed)
	}
}

// SpokenNow answers what is being said, for anything inside the harness that needs it.
func (p *Plugin) SpokenNow() Spoken {
	p.spoken.mu.RLock()
	defer p.spoken.mu.RUnlock()
	return p.spoken.mark
}
