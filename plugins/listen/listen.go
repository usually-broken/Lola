// Package listen turns recorded audio into text.
//
// The recogniser is MLX and lives in a Python virtualenv, so the harness cannot link
// it: this plugin starts one resident worker and talks to it over stdin, because
// loading the model per request costs a second and a half and voice cannot afford it.
//
// The plugin owns a route and a process, and gives both back when unmounted. Nothing
// here captures audio: whoever holds the stream records, and posts what it recorded.
package listen

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/routes"
)

// asksBeforeRestart is how many sentences a worker may transcribe before being replaced.
// Loading the recogniser again is a couple of seconds; leaving one running for hours is
// how a machine ends up swapping.
const asksBeforeRestart = 40

// Path is the address this plugin answers.
const Path = "/api/voice/listen"

// defaultWorker is where the recogniser lives on a machine that has one.
func defaultWorker() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "lola-voice", "worker.py")
}

// A machine that can transcribe is not every machine, so both paths are settings
// rather than assumptions: point them at a worker, or the plugin simply says it has
// no ear.
func setting(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func init() {
	plugin.Register("listen", func(d plugin.Deps) (core.Plugin, error) {
		python := setting("LOLA_LISTEN_PYTHON", "")
		if python == "" {
			if home, err := os.UserHomeDir(); err == nil {
				python = filepath.Join(home, "lola-voice", ".venv", "bin", "python")
			}
		}
		return &Plugin{
			routes: d.Routes,
			python: python,
			script: setting("LOLA_LISTEN_SCRIPT", defaultWorker()),
		}, nil
	})
}

// Plugin answers one route with the help of a resident worker.
type Plugin struct {
	routes *routes.Registry
	python string
	script string

	// asks counts transcriptions since the worker started. MLX fragments over time even
	// with its cache released, so the worker is replaced every so often rather than left
	// to grow until somebody notices their machine slowing down.
	asks int

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	replies chan reply
	dead    string
}

// reply is one answer from the worker, or why it could not give one.
type reply struct {
	Text  string `json:"text"`
	MS    int    `json:"ms"`
	Error string `json:"error"`
}

func (p *Plugin) Name() string { return "listen" }

func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindVoice,
		What: "Turns recorded audio into text, using a resident recogniser so a sentence does not pay for a model load.",
	}
}

// Setup starts the worker and offers the route. A worker that will not start is
// reported rather than hidden: a plugin that silently does nothing is worse than one
// that says why.
func (p *Plugin) Setup() (func() error, error) {
	p.replies = make(chan reply, 8)

	if err := p.start(); err != nil {
		p.dead = err.Error()
	}

	if p.routes == nil {
		return func() error { p.stop(); return nil }, nil
	}
	if err := p.routes.Handle(Path, http.HandlerFunc(p.hear)); err != nil {
		p.stop()
		return nil, err
	}

	return func() error {
		if p.routes != nil {
			p.routes.Remove(Path)
		}
		p.stop()
		return nil
	}, nil
}

// start launches the worker and waits for it to say it is ready.
func (p *Plugin) start() error {
	if p.python == "" || p.script == "" {
		return fmt.Errorf("no worker configured: set LOLA_LISTEN_PYTHON and LOLA_LISTEN_SCRIPT")
	}
	if _, err := os.Stat(p.script); err != nil {
		return fmt.Errorf("worker not found at %s", p.script)
	}

	cmd := exec.Command(p.python, p.script)
	cmd.Stderr = os.Stderr // a crash explains itself where someone will see it

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("worker stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("worker stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start worker: %w", err)
	}

	p.cmd, p.stdin = cmd, stdin

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 16*1024), 1<<20)

	ready := make(chan error, 1)
	go func() {
		first := true
		for scanner.Scan() {
			line := scanner.Bytes()
			var probe struct {
				Ready bool   `json:"ready"`
				Error string `json:"error"`
			}
			if json.Unmarshal(line, &probe) != nil {
				continue
			}
			if first && probe.Ready {
				first = false
				ready <- nil
				continue
			}
			var r reply
			if json.Unmarshal(line, &r) == nil {
				p.replies <- r
			}
		}
		if first {
			ready <- fmt.Errorf("worker exited before it was ready")
		}
	}()

	select {
	case err := <-ready:
		return err
	case <-time.After(90 * time.Second):
		// The first load reads weights from disk; slower than that is broken.
		return fmt.Errorf("worker did not become ready")
	}
}

// stop closes the worker. Best effort: it is going away either way.
func (p *Plugin) stop() {
	if p.stdin != nil {
		p.stdin.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
}

// hear takes audio, writes it down, and asks the worker what was said.
func (p *Plugin) hear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "listen: POST only", http.StatusMethodNotAllowed)
		return
	}
	if p.dead != "" {
		http.Error(w, "listen: "+p.dead, http.StatusServiceUnavailable)
		return
	}

	audio, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil || len(audio) == 0 {
		http.Error(w, "listen: no audio", http.StatusBadRequest)
		return
	}

	file, err := os.CreateTemp("", "lola-listen-*.wav")
	if err != nil {
		http.Error(w, "listen: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.Remove(file.Name())

	if _, err := file.Write(audio); err != nil {
		file.Close()
		http.Error(w, "listen: "+err.Error(), http.StatusInternalServerError)
		return
	}
	file.Close()

	answer, err := p.ask(file.Name())
	if err != nil {
		http.Error(w, "listen: "+err.Error(), http.StatusBadGateway)
		return
	}
	if answer.Error != "" {
		http.Error(w, "listen: "+answer.Error, http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"text": answer.Text,
		"ms":   answer.MS,
	})
}

// ask sends one path and waits for its answer. One at a time: a worker reading stdin
// cannot be asked two things at once and give the answers back in order.
func (p *Plugin) ask(path string) (reply, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stdin == nil {
		return reply{}, fmt.Errorf("no worker running")
	}
	if _, err := fmt.Fprintln(p.stdin, path); err != nil {
		return reply{}, fmt.Errorf("worker is gone: %w", err)
	}

	select {
	case r := <-p.replies:
		p.asks++
		if p.asks >= asksBeforeRestart {
			// Reload costs about two seconds; an unbounded worker costs the machine.
			p.stop()
			if err := p.start(); err != nil {
				p.dead = err.Error()
			}
			p.asks = 0
		}
		return r, nil
	case <-time.After(60 * time.Second):
		return reply{}, fmt.Errorf("worker did not answer")
	}
}

var _ core.Plugin = (*Plugin)(nil)
