// Package listen turns recorded audio into text by asking a transcription endpoint.
//
// It holds no model of its own. Where the audio goes is a setting, and an install that
// has not set one says so plainly instead of failing in a way that looks like a broken
// microphone.
//
// Nothing here captures audio: whoever holds the stream records, and posts what it
// recorded.
package listen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/routes"
)

// Path is the address this plugin answers: POST audio, get text.
const Path = "/api/voice/listen"

// transcriptionPath is the shape every OpenAI-compatible transcription service speaks.
const transcriptionPath = "/audio/transcriptions"

func init() {
	plugin.Register("listen", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{routes: d.Routes}, nil
	})
}

// Plugin hears.
type Plugin struct {
	routes *routes.Registry

	// client gives a slow endpoint room to finish. A transcription of a sentence takes a
	// second or two; cutting it off at a few hundred milliseconds would be worse than
	// waiting.
	client *http.Client
}

func (p *Plugin) Name() string { return "listen" }

func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindVoice,
		What: "Turns recorded audio into text, through a transcription endpoint you set.",
	}
}

// Setup offers the route and gives it back when unmounted.
func (p *Plugin) Setup() (func() error, error) {
	if p.routes == nil {
		return func() error { return nil }, nil
	}
	if p.client == nil {
		p.client = &http.Client{Timeout: 60 * time.Second}
	}
	if err := p.routes.Handle(Path, http.HandlerFunc(p.hear)); err != nil {
		return nil, err
	}
	return func() error {
		p.routes.Remove(Path)
		return nil
	}, nil
}

// hear sends the recording on and answers with what was said.
func (p *Plugin) hear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "listen: POST audio", http.StatusMethodNotAllowed)
		return
	}

	// The client is normally built in Setup, but a plugin with no route to serve never
	// gets that far, and hearing should not depend on being mounted to a page.
	if p.client == nil {
		p.client = &http.Client{Timeout: 60 * time.Second}
	}

	settings := loadSettings()
	if settings.URL == "" {
		http.Error(w, "listen: no transcription endpoint set — add one in Settings, Voice", http.StatusServiceUnavailable)
		return
	}

	audio, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		http.Error(w, "listen: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(audio) == 0 {
		http.Error(w, "listen: empty recording", http.StatusBadRequest)
		return
	}

	started := time.Now()
	text, err := p.transcribe(settings, audio)
	if err != nil {
		http.Error(w, "listen: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"text": strings.TrimSpace(text),
		"ms":   time.Since(started).Milliseconds(),
	})
}

// transcribe posts the audio as multipart, which is what these services expect: a file
// field, a model name, and JSON back with the text in it.
func (p *Plugin) transcribe(settings settings, audio []byte) (string, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)

	file, err := form.CreateFormFile("file", "audio.wav")
	if err != nil {
		return "", err
	}
	if _, err := file.Write(audio); err != nil {
		return "", err
	}
	if settings.Model != "" {
		if err := form.WriteField("model", settings.Model); err != nil {
			return "", err
		}
	}
	if err := form.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint(settings.URL), &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	if settings.Key != "" {
		req.Header.Set("Authorization", "Bearer "+settings.Key)
	}

	res, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	reply, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return "", fmt.Errorf("endpoint said %d: %s", res.StatusCode, strings.TrimSpace(string(reply)))
	}

	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(reply, &out); err != nil {
		return "", fmt.Errorf("endpoint did not answer with text: %w", err)
	}
	return out.Text, nil
}

// endpoint accepts a base URL or the full path, because both get pasted.
func endpoint(url string) string {
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	if strings.HasSuffix(url, transcriptionPath) {
		return url
	}
	return url + transcriptionPath
}

// settings is the part of the voice settings this plugin needs. It reads the same file
// the voice section writes, so there is one place to say where audio goes.
type settings struct {
	URL   string `json:"listenUrl"`
	Model string `json:"listenModel"`
	Key   string `json:"listenKey"`
}

// loadSettings reads ~/.lola/voice.json. A missing or broken file means "not configured",
// which is a thing to say clearly, not an error to shout about.
func loadSettings() settings {
	home, err := os.UserHomeDir()
	if err != nil {
		return settings{}
	}
	body, err := os.ReadFile(filepath.Join(home, ".lola", "voice.json"))
	if err != nil {
		return settings{}
	}
	var out settings
	if err := json.Unmarshal(body, &out); err != nil {
		return settings{}
	}
	out.URL = strings.TrimSpace(out.URL)
	out.Model = strings.TrimSpace(out.Model)
	out.Key = strings.TrimSpace(out.Key)
	return out
}

var _ core.Plugin = (*Plugin)(nil)
