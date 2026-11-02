// Package speak turns text into speech.
//
// It is the smallest useful half of voice: give it a sentence, get audio back.
// Splitting a reply into sentences belongs to whoever holds the stream, because
// that is where the text already arrives piece by piece. This only knows how to
// say one sentence at a time.
package speak

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/routes"
)

// Path is the address this plugin answers.
const Path = "/api/voice/speak"

// SentencesPath says where a reply would be spoken in pieces, without speaking it.
// The splitting rule is a rule about language, so it is asked of the plugin rather
// than re-implemented wherever a reply happens to be played.
const SentencesPath = "/api/voice/sentences"

// Defaults describe a stock Ollama on the same machine: the speech model is small
// enough to run beside everything else, and the endpoint is the OpenAI-shaped one
// every provider speaks now.
const (
	DefaultEndpoint = "http://127.0.0.1:11434/v1/audio/speech"
	DefaultModel    = "phonon"
	DefaultVoice    = "Everly"
)

func init() {
	plugin.Register("speak", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{routes: d.Routes}, nil
	})
}

// Plugin speaks text through a speech endpoint.
type Plugin struct {
	routes *routes.Registry

	// client gives a slow sentence room to finish. A tight timeout would cut it
	// off mid-word, which sounds worse than waiting.
	client *http.Client

	// spoken is what is being said right now. Nothing reads it yet; it is here so
	// that interrupting has something to interrupt from.
	spoken spokenState
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "speak" }

// About says what this is, so the plugins screen can group it.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindVoice,
		What: "Says a sentence out loud, so an interface can speak a reply instead of printing it.",
	}
}

// Setup answers one address, and gives it back when unmounted: an unmounted
// plugin must not leave a path behind that says it is still here.
func (p *Plugin) Setup() (func() error, error) {
	if p.routes == nil {
		return func() error { return nil }, nil
	}
	if p.client == nil {
		p.client = &http.Client{Timeout: 60 * time.Second}
	}
	if err := p.routes.Handle(Path, http.HandlerFunc(p.speak)); err != nil {
		return nil, err
	}
	if err := p.routes.Handle(SentencesPath, http.HandlerFunc(p.sentences)); err != nil {
		p.routes.Remove(Path)
		return nil, err
	}
	if err := p.routes.Handle(SpokenPath, http.HandlerFunc(p.HandleSpoken)); err != nil {
		p.routes.Remove(Path)
		p.routes.Remove(SentencesPath)
		return nil, err
	}
	return func() error {
		p.routes.Remove(Path)
		p.routes.Remove(SentencesPath)
		p.routes.Remove(SpokenPath)
		return nil
	}, nil
}

// Setting is where one of the defaults can be overridden, so a different voice or
// a different machine does not need a code change.
func setting(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

// speak answers with the audio itself rather than a URL: a sentence is small, and
// a second round trip to collect it would cost more than sending it.
func (p *Plugin) speak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "speak: POST only", http.StatusMethodNotAllowed)
		return
	}

	var ask struct {
		Text  string `json:"text"`
		Voice string `json:"voice,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&ask); err != nil {
		http.Error(w, "speak: "+err.Error(), http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(ask.Text)
	if text == "" {
		http.Error(w, "speak: nothing to say", http.StatusBadRequest)
		return
	}

	// The file wins over the environment, and the environment over the default: a
	// person changing a setting should not have to restart anything.
	cfg := settingsFile()

	voice := strings.TrimSpace(ask.Voice)
	if voice == "" {
		voice = firstNonEmpty(cfg.Voice, setting("LOLA_SPEAK_VOICE", DefaultVoice))
	}

	body, err := json.Marshal(map[string]string{
		"model": firstNonEmpty(cfg.Model, setting("LOLA_SPEAK_MODEL", DefaultModel)),
		"voice": voice,
		"input": text,
	})
	if err != nil {
		http.Error(w, "speak: "+err.Error(), http.StatusInternalServerError)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		firstNonEmpty(cfg.Endpoint, setting("LOLA_SPEAK_ENDPOINT", DefaultEndpoint)), bytes.NewReader(body))
	if err != nil {
		http.Error(w, "speak: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		http.Error(w, "speak: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		http.Error(w, fmt.Sprintf("speak: endpoint said %s: %s", resp.Status, strings.TrimSpace(string(detail))),
			http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "no-store")
	io.Copy(w, resp.Body)
}

// sentences answers with the pieces a reply would be spoken in.
func (p *Plugin) sentences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "speak: POST only", http.StatusMethodNotAllowed)
		return
	}

	var ask struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&ask); err != nil {
		http.Error(w, "speak: "+err.Error(), http.StatusBadRequest)
		return
	}

	parts := Split(ask.Text)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"sentences": parts,
		"count":     len(parts),
	})
}

// spokenSettings is the part of the voice settings this plugin cares about.
type spokenSettings struct {
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	Voice    string `json:"voice"`
}

// settingsFile is the same file the voice plugin writes, read fresh each call so a
// change lands on the next sentence rather than the next restart.
func settingsFile() spokenSettings {
	var out spokenSettings

	home, err := os.UserHomeDir()
	if err != nil {
		return out
	}
	body, err := os.ReadFile(filepath.Join(home, ".lola", "voice.json"))
	if err != nil {
		return out
	}
	json.Unmarshal(body, &out)
	return out
}

// firstNonEmpty takes the first value that says something.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
