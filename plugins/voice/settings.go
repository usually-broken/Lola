package voice

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// Settings are the voice settings, in one place, kept in ~/.lola/voice.json.
//
// One file rather than three env vars and no settings screen: the endpoint, the model,
// the voice and the colour of the ball are all the same decision — what voice mode
// sounds and looks like — and they belong where a person can change them.
type Settings struct {
	// Endpoint is the speech endpoint. Any OpenAI-shaped /audio/speech will do, so this
	// is not tied to running a model locally.
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	Voice    string `json:"voice"`

	// The ball. Core is the middle, cloud is the white (or not white) drifting through
	// it, rim is the edge.
	// The ear. Empty means "not configured yet", which is said plainly when somebody tries
	// to use it rather than being invented.
	ListenURL   string `json:"listenUrl"`
	ListenModel string `json:"listenModel"`
	ListenKey   string `json:"listenKey"`

	OrbCore  string `json:"orbCore"`
	OrbCloud string `json:"orbCloud"`
	OrbRim   string `json:"orbRim"`
}

// SettingsPath is where the file lives.
func SettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".lola", "voice.json")
}

// DefaultSettings are what somebody who has changed nothing gets: a local model and a
// blue sky.
func DefaultSettings() Settings {
	return Settings{
		Endpoint: "http://127.0.0.1:11434/v1/audio/speech",
		Model:    "phonon",
		Voice:    "Everly",
		OrbCore:  "#6ba6e8",
		OrbCloud: "#ffffff",
		OrbRim:   "rgba(255,255,255,0.55)",
	}
}

// LoadSettings reads the file, falling back to the defaults for anything missing. A
// broken file is not a reason to have no voice: it is a reason to have the default one.
func LoadSettings() Settings {
	out := DefaultSettings()

	path := SettingsPath()
	if path == "" {
		return out
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return DefaultSettings()
	}

	// An empty field means "unchanged", not "blank".
	base := DefaultSettings()
	if out.Endpoint == "" {
		out.Endpoint = base.Endpoint
	}
	if out.Model == "" {
		out.Model = base.Model
	}
	if out.Voice == "" {
		out.Voice = base.Voice
	}
	if out.OrbCore == "" {
		out.OrbCore = base.OrbCore
	}
	if out.OrbCloud == "" {
		out.OrbCloud = base.OrbCloud
	}
	if out.OrbRim == "" {
		out.OrbRim = base.OrbRim
	}
	return out
}

// SaveSettings writes the file, making the folder if it is not there yet.
func SaveSettings(s Settings) error {
	path := SettingsPath()
	if path == "" {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

// SettingsPathRoute is where the settings screen reads and writes.
const SettingsPathRoute = "/api/voice/settings"

// SettingsScript wires the section's controls, served so the section can bring its own
// behaviour the way it brings its own markup.
const SettingsScript = "/api/voice/settings.js"

// settingsSection is the panel: what voice mode is, and what it looks like.
const settingsSection = `<style>
  .voice-grid { display: grid; gap: 10px; }
</style>
<h2>Voice</h2>
<p class="intro">What reads your replies aloud, and what the ball looks like while it listens.</p>

<div class="field">
  <div class="label">Speech endpoint</div>
  <div class="why">Any OpenAI-shaped <code>/audio/speech</code>. Local by default; a hosted service works too.</div>
  <input id="voiceEndpoint" type="text" spellcheck="false" placeholder="http://127.0.0.1:11434/v1/audio/speech">
</div>

<div class="field">
  <div class="label">Model</div>
  <div class="why">The speech model at that endpoint.</div>
  <input id="voiceModel" type="text" spellcheck="false" placeholder="phonon">
</div>

<div class="field">
  <div class="label">Voice</div>
  <div class="why">Which voice it speaks in.</div>
  <input id="voiceName" type="text" spellcheck="false" placeholder="Everly">
</div>

<div class="field">
  <div class="label">Transcription endpoint</div>
  <div class="why">Where recordings go to be turned into text. Any OpenAI-shaped <code>/audio/transcriptions</code>. Empty means voice mode has no ear.</div>
  <input id="voiceListenUrl" type="text" spellcheck="false" placeholder="https://api.example.com/v1">
</div>

<div class="field">
  <div class="label">Transcription model</div>
  <div class="why">The model at that endpoint, sent with each recording.</div>
  <input id="voiceListenModel" type="text" spellcheck="false" placeholder="whisper-large-v3">
</div>

<div class="field">
  <div class="label">Ball</div>
  <div class="why">The middle, the cloud drifting through it, and the rim.</div>
  <div class="voice-grid" style="grid-template-columns: repeat(3, minmax(0, 1fr));">
    <label class="why">Core <input id="voiceOrbCore" type="text" spellcheck="false" placeholder="#6ba6e8"></label>
    <label class="why">Cloud <input id="voiceOrbCloud" type="text" spellcheck="false" placeholder="#ffffff"></label>
    <label class="why">Rim <input id="voiceOrbRim" type="text" spellcheck="false" placeholder="rgba(255,255,255,0.55)"></label>
  </div>
</div>

<div class="field">
  <button id="voiceSave" type="button">Save voice settings</button>
  <span id="voiceNote" class="why"></span>
</div>

<script src="/api/voice/settings.js?v=3"></script>`

// settingsHandler reads and writes the file.
func (p *Plugin) settingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LoadSettings())

	case http.MethodPost:
		var ask Settings
		if err := json.NewDecoder(r.Body).Decode(&ask); err != nil {
			http.Error(w, "voice: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := SaveSettings(ask); err != nil {
			http.Error(w, "voice: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"saved": true})

	default:
		http.Error(w, "voice: GET or POST", http.StatusMethodNotAllowed)
	}
}
