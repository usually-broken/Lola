package voice

import (
	_ "embed"
	"net/http"
)

// The ball is the plugin's own: its markup and the code that animates it are served
// from here, so the page draws what it is handed and knows nothing about microphones,
// analysers or sentence pauses. Same rule the settings sections follow — a section
// supplies its panel rather than a copy of it.

//go:embed orb.js
var orbScript []byte

//go:embed settings.js
var settingsScript []byte

// PlaceVoiceMode is where the ball goes: its own place, above the chips.
const PlaceVoiceMode = "voice-mode"

// ScriptPath serves the behaviour, because markup inserted into a page cannot bring
// working script with it.
const ScriptPath = "/api/voice/ball.js"

// ScriptVersion is part of the URL the page loads. A script served from a fixed address
// can be cached no matter what its headers say, and a cached copy of this one looks
// exactly like a bug in the harness.
const ScriptVersion = "1"

// orbMarkup is the row itself. A fragment, so it is the harness's own markup rather
// than a stranger's: plugins are compiled in.
const orbMarkup = `<div id="orbBackdrop" class="orb-row" style="display:none;align-items:center;justify-content:center;gap:10px;padding:8px 0 2px;">
  <div class="orb-wrap" style="display:grid;justify-items:center;gap:2px;">
    <canvas id="orbCanvas" width="300" height="300" style="width:104px;height:104px;display:block;"></canvas>
    <div id="orbHeard" style="max-width:340px;text-align:center;color:#7d8590;font-size:0.78rem;min-height:1em;opacity:0.85;"></div>
  </div>
  <button id="orbExit" type="button" style="font:inherit;font-size:0.76rem;letter-spacing:0.04em;color:var(--muted,#5a6472);background:var(--panel,#eef1f5);border:1px solid var(--line,#d7dbe0);border-radius:999px;padding:5px 12px;cursor:pointer;">exit</button>
</div>
<script src="/api/voice/ball.js?v=5"></script>
`

// script hands the page the behaviour that belongs to that markup.
func (p *Plugin) script(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(orbScript)
}

// serveSettingsScript hands the section its behaviour.
func (p *Plugin) serveSettingsScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(settingsScript)
}
