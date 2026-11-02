// Package settingsscreen serves what the harness is set to, and applies changes.
//
// The page held both halves until now, which meant the frontend knew about the
// settings service and the keeper behind it. This owns them instead: it reads
// and writes through the seam on Deps, so the page only has to draw a form.
//
// The addresses are the ones the page already asks, so nothing in the frontend
// had to change to stop owning this.
package settingsscreen

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/routes"
)

func init() {
	plugin.Register("settingsscreen", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{routes: d.Routes, settings: d.Settings}, nil
	})
}

const (
	// Path answers with what the harness is set to.
	Path = "/api/settings"

	// SetPath applies one change.
	SetPath = "/api/settings/set"
)

// Plugin owns both paths and holds the seam it acts through.
type Plugin struct {
	routes   *routes.Registry
	settings plugin.Settings
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "settingsscreen" }

// About says what this is, so the plugins screen can group it and say what it
// is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindUI,
		What: "Reads and changes what the harness is set to: model, mode, folders and guard rules.",
	}
}

// Setup offers both paths, and takes them back when it is unmounted.
func (p *Plugin) Setup() (func() error, error) {
	if p.routes == nil {
		return func() error { return nil }, nil
	}
	if err := p.routes.Handle(Path, p.read); err != nil {
		return nil, err
	}
	if err := p.routes.Handle(SetPath, p.write); err != nil {
		return nil, err
	}
	return func() error {
		p.routes.Remove(Path)
		p.routes.Remove(SetPath)
		return nil
	}, nil
}

func (p *Plugin) read(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "settings: GET only", http.StatusMethodNotAllowed)
		return
	}
	if p.settings == nil {
		http.Error(w, "settings: nothing here offers settings", http.StatusServiceUnavailable)
		return
	}

	view := p.settings.Read()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Model       string   `json:"model"`
		Mode        string   `json:"mode"`
		Source      string   `json:"source"`
		Workspace   []string `json:"workspace"`
		Protected   []string `json:"protected"`
		Gated       []string `json:"gated"`
		Dangerous   []string `json:"dangerous"`
		ConfigPath  string   `json:"configPath"`
		SessionsDir string   `json:"sessionsDir"`
	}{
		Model:       view.Model,
		Mode:        view.Mode,
		Source:      view.Source,
		Workspace:   view.Workspace,
		Protected:   view.Protected,
		Gated:       view.Gated,
		Dangerous:   view.Dangerous,
		ConfigPath:  view.ConfigPath,
		SessionsDir: view.SessionsDir,
	})
}

func (p *Plugin) write(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "settings: POST only", http.StatusMethodNotAllowed)
		return
	}
	if p.settings == nil {
		http.Error(w, "settings: nothing here offers settings", http.StatusServiceUnavailable)
		return
	}

	// The page sends the settings it wants changed and leaves the rest out, so
	// a form can save one control without posting everything else back.
	var patch struct {
		Model     *string   `json:"model"`
		Mode      *string   `json:"mode"`
		Source    *string   `json:"source"`
		Workspace *[]string `json:"workspace"`
		Protected *[]string `json:"protected"`
		Gated     *[]string `json:"gated"`
		Dangerous *[]string `json:"dangerous"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		http.Error(w, "settings: bad request", http.StatusBadRequest)
		return
	}

	var errs []string
	// A string is sent as itself; only a list or an object is encoded. Wrapping
	// a plain string in quotes saved the quotes, so a model name arrived as
	// "model" rather than model, and nothing reading it back could use it.
	applyText := func(what, value string) {
		if err := p.settings.Write(what, value); err != nil {
			errs = append(errs, what+": "+err.Error())
		}
	}
	applyJSON := func(what string, value any) {
		encoded, err := json.Marshal(value)
		if err != nil {
			errs = append(errs, what+": "+err.Error())
			return
		}
		applyText(what, string(encoded))
	}

	if patch.Model != nil {
		applyText("model", *patch.Model)
	}
	if patch.Mode != nil {
		applyText("mode", *patch.Mode)
	}
	if patch.Source != nil {
		applyText("source", *patch.Source)
	}
	if patch.Workspace != nil {
		applyJSON("workspace", *patch.Workspace)
	}
	if patch.Protected != nil || patch.Gated != nil || patch.Dangerous != nil {
		rules := struct {
			Protected []string `json:"protected"`
			Gated     []string `json:"gated"`
			Dangerous []string `json:"dangerous"`
		}{}
		if patch.Protected != nil {
			rules.Protected = *patch.Protected
		}
		if patch.Gated != nil {
			rules.Gated = *patch.Gated
		}
		if patch.Dangerous != nil {
			rules.Dangerous = *patch.Dangerous
		}
		applyJSON("guard", rules)
	}

	if len(errs) > 0 {
		http.Error(w, "settings: "+strings.Join(errs, "; "), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// Assert at compile time that this plugin satisfies the seam every plugin does.
var _ core.Plugin = (*Plugin)(nil)
