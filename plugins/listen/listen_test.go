package listen

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// withSettings points the plugin at a temporary home with a voice.json in it, and puts
// the real one back afterwards.
func withSettings(t *testing.T, body string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".lola"), 0o755); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		if err := os.WriteFile(filepath.Join(home, ".lola", "voice.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// The plugin's job is the request it sends: a multipart file, a model name, and the
// endpoint's text read back out.
func TestHearSendsAudioAndReturnsText(t *testing.T) {
	var gotModel, gotFile string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Errorf("posted to %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("not multipart: %v", err)
			return
		}
		gotModel = r.FormValue("model")
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("no file field: %v", err)
			return
		}
		defer file.Close()
		audio, _ := io.ReadAll(file)
		gotFile = string(audio)

		json.NewEncoder(w).Encode(map[string]string{"text": " hello there "})
	}))
	defer server.Close()

	withSettings(t, `{"listenUrl":"`+server.URL+`/v1","listenModel":"whisper-large-v3"}`)

	p := &Plugin{}
	if _, err := p.Setup(); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	p.hear(rec, httptest.NewRequest(http.MethodPost, Path, stringReader("RIFFfake audio")))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["text"] != "hello there" {
		t.Fatalf("text = %q, want trimmed", out["text"])
	}
	if gotModel != "whisper-large-v3" {
		t.Fatalf("model field = %q", gotModel)
	}
	if gotFile != "RIFFfake audio" {
		t.Fatalf("audio arrived as %q", gotFile)
	}
}

// A full endpoint URL is as valid as a base, because people paste both.
func TestEndpointAcceptsBothShapes(t *testing.T) {
	if got := endpoint("https://api.example.com/v1"); got != "https://api.example.com/v1/audio/transcriptions" {
		t.Fatalf("base URL became %q", got)
	}
	if got := endpoint("https://api.example.com/v1/audio/transcriptions/"); got != "https://api.example.com/v1/audio/transcriptions" {
		t.Fatalf("full URL became %q", got)
	}
}

// With nothing configured, it says what is missing rather than pretending to listen.
func TestHearWithoutAnEndpointSaysSo(t *testing.T) {
	withSettings(t, "")
	p := &Plugin{}
	p.Setup()

	rec := httptest.NewRecorder()
	p.hear(rec, httptest.NewRequest(http.MethodPost, Path, stringReader("audio")))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

type textReader struct {
	s string
	i int
}

func stringReader(s string) *textReader { return &textReader{s: s} }

func (r *textReader) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}
