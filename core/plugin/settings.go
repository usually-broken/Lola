package plugin

// Settings is what the harness is set to, as a plugin sees it.
//
// It exists so a screen can offer settings without the plugin that draws it
// knowing where any of this comes from — not the settings service, not the
// keeper, not the file. The page has held this until now, which is why settings
// was the one screen that could not move.
//
// Read is a copy rather than a live view: a plugin that draws a screen should
// not be holding a lock while it does, and a snapshot cannot change underneath
// a form that is being filled in.
type Settings interface {
	// Read is the current values, as plain data.
	Read() SettingsView

	// Write applies one change, naming what is being changed:
	//
	//	"model"     the model to run against
	//	"mode"      how much the harness asks before it acts
	//	"source"    where tasks are read from
	//	"workspace"  the folders a run may touch     (value: JSON list)
	//	"guard"      what is protected, gated, dangerous (value: JSON object)
	//
	// An error means it was not applied, and the caller can say so rather than
	// pretending it worked.
	Write(what, value string) error
}

// SettingsView is one reading of the harness's settings.
//
// It is deliberately plain: a screen shows strings and lists, and nothing here
// needs a struct from an internal package to be understood.
type SettingsView struct {
	Model       string
	Mode        string
	Source      string
	Workspace   []string
	Protected   []string
	Gated       []string
	Dangerous   []string
	ConfigPath  string
	SessionsDir string
}
