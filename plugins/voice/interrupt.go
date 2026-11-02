package voice

import (
	"encoding/json"
	"net/http"
	"strings"
)

// InterruptPath is where a frontend says it cut the harness off, and where the note
// it produces can be read back.
const InterruptPath = "/api/voice/interrupt"

// cut is a sentence that was being said when someone started talking over it.
//
// The note is single use: it belongs to the next turn, because "you were cut off" is
// only true once. After that the conversation has moved on and repeating it would be
// the model apologising for something nobody remembers.
type cut struct {
	Sentence string `json:"sentence,omitempty"`
	Index    int    `json:"index,omitempty"`
	At       string `json:"at,omitempty"`
}

// HandleInterrupt records a cut, or answers with the one waiting to be explained.
func (p *Plugin) HandleInterrupt(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		p.mu.Lock()
		note := p.cut
		p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(note)

	case http.MethodPost:
		var in cut
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "voice: "+err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(in.Sentence) == "" {
			http.Error(w, "voice: an interruption needs the sentence it cut into", http.StatusBadRequest)
			return
		}
		p.mu.Lock()
		// Kept, not spent. A cut is part of what the conversation knows: the next turn needs
		// it, and so does the one after, because an answer that stops mid-thought is
		// otherwise unexplainable. Three is enough to explain a conversation without
		// filling the prompt with old stops.
		p.cuts = append(p.cuts, strings.TrimSpace(in.Sentence))
		if len(p.cuts) > 3 {
			p.cuts = p.cuts[len(p.cuts)-3:]
		}
		p.cut = in
		p.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "GET or POST", http.StatusMethodNotAllowed)
	}
}

// cutHistory is every recent cut, said as one line, so a later turn knows an answer was
// stopped and where.
func cutHistory(cuts []string) string {
	if len(cuts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nEarlier in this conversation you were cut off while speaking. ")
	for _, c := range cuts {
		b.WriteString("You had got as far as: \"" + c + "\". ")
	}
	b.WriteString("Do not continue those answers, and do not apologise for the interruptions. ")
	return b.String()
}

// takeCut returns the pending note and clears it, so it is said once.
func (p *Plugin) takeCut() cut {
	p.mu.Lock()
	defer p.mu.Unlock()
	note := p.cut
	p.cut = cut{}
	return note
}

// cutNote is the line the model gets after being spoken over. It says what was said,
// where it stopped, and what to do instead of carrying on.
func cutNote(c cut) string {
	if strings.TrimSpace(c.Sentence) == "" {
		return ""
	}
	return "\n\nYou were speaking and they cut in. What they heard of you ended at: \"" +
		strings.TrimSpace(c.Sentence) + "\". Do not continue it, do not apologise for being " +
		"interrupted, and do not restate it. Answer what they just said."
}
