// Package guard decides whether a tool call may run, must be refused, or must
// stop and ask a person first.
//
// The decision is made by the harness, not by the model. It is a check in code
// that runs before a tool executes, so it cannot be talked around or reasoned
// away — a model can be persuaded out of an instruction, but not out of a check
// that stops the call before it happens.
package guard

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// Decision is what the guard says about a tool call.
type Decision int

const (
	// Allow means run it.
	Allow Decision = iota

	// Ask means stop and get a person's consent before running.
	Ask

	// Deny means never run it, consent or not.
	Deny
)

func (d Decision) String() string {
	switch d {
	case Allow:
		return "allow"
	case Ask:
		return "ask"
	case Deny:
		return "deny"
	}
	return "unknown"
}

// Approval is a person's answer to an Ask.
type Approval int

const (
	// Approved means go ahead.
	Approved Approval = iota

	// Denied means do not.
	Denied

	// Unavailable means there is nobody to ask — a plain command-line run, for
	// instance. Treated as refused, because the safe default is no.
	Unavailable
)

// Mode is how much consent the harness asks for before it acts. It is the one
// setting that decides whether a person is in the loop at all.
type Mode string

const (
	// ModeWorkspace is the default: work inside the workspace runs, and the
	// gated paths and dangerous commands still stop and ask.
	ModeWorkspace Mode = "workspace"

	// ModeAsk stops and asks before every tool call, whoever wrote the tool.
	//
	// It is the mode for a tree you did not write. A policy that covers tools by
	// name cannot keep up with plugins, so in this mode the name is not
	// consulted at all: every call goes to a person.
	ModeAsk Mode = "ask"

	// ModeOpen runs without asking. The protected paths are still refused:
	// that is a wall, not a preference.
	ModeOpen Mode = "open"
)

// Valid reports whether the mode is one we know.
func (m Mode) Valid() bool {
	switch m {
	case ModeWorkspace, ModeAsk, ModeOpen:
		return true
	}
	return false
}

// Modes lists the modes in the order they loosen: writes inside the workspace
// without being asked, every call approved first, then nothing asked at all.
//
// A settings screen offering the choice reads the list from here rather than
// keeping its own copy, which goes stale the moment a mode is added.
func Modes() []Mode { return []Mode{ModeWorkspace, ModeAsk, ModeOpen} }

// ParseMode reads a mode from a string. An unknown name is an error rather than
// a silent fallback, so a typo cannot quietly loosen the guard.
func ParseMode(s string) (Mode, error) {
	m := Mode(s)
	if !m.Valid() {
		return "", fmt.Errorf("guard: unknown mode %q", s)
	}
	return m, nil
}

// Approver is asked when the guard returns Ask. A frontend supplies one; the
// TUI can put a question on screen, and the command line supplies one that
// always answers Unavailable.
type Approver interface {
	// Approve is called with a plain description of what wants to happen.
	Approve(ctx context.Context, request string) Approval
}

// RefuseAll is an Approver for runs with nobody watching: it always declines.
// This is what makes the command line refuse by default.
type RefuseAll struct{}

// Approve always returns Unavailable.
func (RefuseAll) Approve(context.Context, string) Approval { return Unavailable }

// Policy decides what may happen. It is consulted before every tool call.
type Policy interface {
	// Check decides on one tool call, given its name and raw arguments.
	Check(toolName string, args json.RawMessage) Decision
}

// Rules is the built-in policy.
//
// It refuses the harness's own private state outright, and gates the things a
// person should consent to. Everything else is allowed.
type Rules struct {
	// mu guards the lists, since the interface can change the source while the
	// agent's tools are checking them.
	mu sync.RWMutex

	// Protected are paths that may never be touched, whatever the user says.
	// The session store and profiles live here.
	Protected []string

	// Gated are paths that need consent before they are written or removed.
	// The config and the plugins live here.
	Gated []string

	// Source is where the harness's own source lives, if the user told us.
	// Empty means the rule is not armed.
	Source string

	// Dangerous are shell commands that need consent.
	Dangerous []string

	// mode is how much consent to ask for. Empty reads as the default, so a
	// Rules built before modes existed behaves the way it always did.
	mode Mode
}

// Default returns the built-in rules for a harness home, plus a source tree if
// the user has told us where theirs is (empty means that rule is not armed).
func Default(homeDir, sourceDir string) *Rules {
	r := &Rules{
		Protected: []string{
			filepath.Join(homeDir, "sessions"),
			filepath.Join(homeDir, "profiles"),
		},
		Gated: []string{
			filepath.Join(homeDir, "lola.yaml"),
		},
		Dangerous: []string{
			"rm", "rmdir", "mv", "dd", "mkfs", "shred",
			"chmod", "chown", "sudo", "kill", "pkill", "truncate",
		},
	}
	// The whole source tree is gated, when we know where it is.
	r.Source = sourceDir
	return r
}

// Spec is a guard policy a config can supply. An empty list means "keep the
// built-in default for that list".
type Spec struct {
	Protected []string
	Gated     []string
	Dangerous []string
}

// FromSpec builds the rules from the defaults for a home, with any lists the
// spec supplies replacing them.
//
// The source tree, when known, stays gated even if the spec lists its own gated
// paths: editing your own source is the one thing not to lose the guard on.
func FromSpec(homeDir, sourceDir string, spec Spec) *Rules {
	r := Default(homeDir, sourceDir)
	if len(spec.Protected) > 0 {
		r.Protected = spec.Protected
	}
	if len(spec.Gated) > 0 {
		r.Gated = spec.Gated
	}
	if len(spec.Dangerous) > 0 {
		r.Dangerous = spec.Dangerous
	}
	// The source stays gated whatever the spec says: editing your own source is
	// the one thing not to lose the guard on.
	r.Source = sourceDir
	return r
}

// SetSource changes where the harness's own source lives, at runtime. An empty
// string disarms the rule.
func (r *Rules) SetSource(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Source = dir
}

// SourceOf returns the configured source directory, if any.
func (r *Rules) SourceOf() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Source
}

// Lists returns copies of the policy lists, for a settings screen.
func (r *Rules) Lists() (protected, gated, dangerous []string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.Protected...),
		append([]string(nil), r.Gated...),
		append([]string(nil), r.Dangerous...)
}

// SetLists replaces the policy lists while the harness is running.
func (r *Rules) SetLists(protected, gated, dangerous []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Protected = protected
	r.Gated = gated
	r.Dangerous = dangerous
}

// ModeOf is the mode in force. An unset value reads as the default rather than
// as "no mode at all".
func (r *Rules) ModeOf() Mode {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.mode == "" {
		return ModeWorkspace
	}
	return r.mode
}

// SetMode changes how much consent the guard asks for, while the harness runs.
func (r *Rules) SetMode(m Mode) error {
	if !m.Valid() {
		return fmt.Errorf("guard: unknown mode %q", string(m))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mode = m
	return nil
}

// Check decides on a tool call.
func (r *Rules) Check(toolName string, args json.RawMessage) Decision {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mode := r.mode

	// A protected path is a wall rather than a preference, and it is decided
	// first, whatever the mode: a mode that asks about everything must not be
	// able to be talked into opening the harness's own private state.
	//
	// Checked for any tool carrying a path, not only the file tools the harness
	// ships with, because a tool from a plugin can reach the same files.
	if path := stringField(args, "path"); path != "" && r.isProtected(path) {
		return Deny
	}

	if mode == ModeOpen {
		return Allow
	}

	// Ask mode puts every call to a person, whatever the tool is called.
	//
	// Deliberately not a list of names. A harness that can be extended has tools
	// this code has never heard of, and a policy that only covers the ones it
	// ships with is a policy that quietly stops covering everything added later
	// — which is not a hypothetical. Before this, anything unnamed fell through
	// to allowed, so a plugin's tool ran without being put to anybody.
	if mode == ModeAsk {
		return Ask
	}

	// Workspace mode: work inside the workspace runs, and the things a person
	// should consent to still stop and ask.
	switch toolName {
	case "write_file", "delete_file", "read_file":
		path := stringField(args, "path")
		if path == "" {
			return Allow // the tool itself will complain about missing arguments
		}
		// Reading changes nothing, so there is nothing to consent to. Writing
		// and deleting both change things — and a delete changes them harder,
		// so it is never the softer of the two.
		if toolName != "read_file" && r.isGated(path) {
			return Ask
		}
		return Allow

	case "run_shell":
		if r.isDangerous(stringField(args, "command")) {
			return Ask
		}
		return Allow
	}

	return Allow
}

// isProtected reports whether a path is in the never-touch set.
func (r *Rules) isProtected(path string) bool {
	return underAny(path, r.Protected)
}

// isGated reports whether a path needs consent: the gated set, or the source.
func (r *Rules) isGated(path string) bool {
	if underAny(path, r.Gated) {
		return true
	}
	if r.Source != "" {
		if _, ok := within(path, r.Source); ok {
			return true
		}
	}
	return false
}

// isDangerous reports whether a shell command needs consent.
func (r *Rules) isDangerous(command string) bool {
	for _, word := range strings.Fields(command) {
		// Look at the command word itself, ignoring a leading path.
		base := filepath.Base(word)
		for _, d := range r.Dangerous {
			if base == d {
				return true
			}
		}
	}
	for _, d := range r.Dangerous {
		// Redirections: any ">>" or ">" in the command is a write.
		if (d == ">" || d == ">>") && strings.Contains(command, d) {
			return true
		}
	}
	return false
}

// underAny reports whether path is inside any of the directories.
func underAny(path string, dirs []string) bool {
	for _, d := range dirs {
		if _, ok := within(path, d); ok {
			return true
		}
	}
	return false
}

// within reports whether path is dir or inside it. Both are resolved through
// symlinks so a link cannot disguise the target.
func within(path, dir string) (string, bool) {
	target, err := realPath(path)
	if err != nil {
		return "", false
	}
	root, err := realPath(dir)
	if err != nil {
		// The guard directory may not exist; fall back to a cleaned absolute.
		root, err = filepath.Abs(dir)
		if err != nil {
			return "", false
		}
	}
	if target == root {
		return root, true
	}
	if strings.HasPrefix(target, root+string(filepath.Separator)) {
		return root, true
	}
	return "", false
}

// realPath resolves a path to an absolute, symlink-free form, tolerating paths
// that do not exist yet by resolving their nearest existing ancestor.
func realPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	existing := abs
	var missing []string
	for {
		if resolved, err := filepath.EvalSymlinks(existing); err == nil {
			return filepath.Join(append([]string{resolved}, missing...)...), nil
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return abs, nil
		}
		missing = append([]string{filepath.Base(existing)}, missing...)
		existing = parent
	}
}

// stringField pulls a string field out of a raw JSON object.
func stringField(raw json.RawMessage, field string) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	s, _ := m[field].(string)
	return s
}

// Describe renders a tool call for a person to decide on.
func Describe(toolName string, args json.RawMessage) string {
	switch toolName {
	case "write_file":
		return fmt.Sprintf("write to %s", stringField(args, "path"))
	case "delete_file":
		return fmt.Sprintf("delete %s", stringField(args, "path"))
	case "read_file":
		return fmt.Sprintf("read %s", stringField(args, "path"))
	case "run_shell":
		return fmt.Sprintf("run: %s", stringField(args, "command"))
	}
	return fmt.Sprintf("use %s", toolName)
}

// compile-time: Rules is a Policy.
var _ Policy = (*Rules)(nil)

// --- the gate ------------------------------------------------------------

// Gate adapts a Policy and an Approver to the tools registry's gate seam. It is
// what the registry consults before every tool call.
//
// The policy decides allow, ask, or deny. On ask, the approver is consulted; if
// no approver is supplied, or it returns Unavailable, the call is refused. That
// is what makes the command line safe by default.
type Gate struct {
	mu sync.RWMutex

	Policy   Policy
	Approver Approver

	// OnDecision, if set, is told what the guard decided, so a frontend can
	// show a record of it.
	OnDecision func(toolName string, decision Decision, reason string)

	// declined remembers what the person has already said no to in this run.
	// Asking twice is an interrogation, not a question: once they have answered,
	// the answer stands, and the model is told so rather than being handed the
	// same request to try again.
	declined map[string]bool
}

// NewGate returns a gate for a policy and an approver. A nil approver refuses
// every gated call.
func NewGate(p Policy, a Approver) *Gate {
	return &Gate{Policy: p, Approver: a}
}

// SetApprover swaps the approver. A frontend that can put a question to a
// person calls this before it runs, replacing the refuse-all default.
func (g *Gate) SetApprover(a Approver) {
	g.mu.Lock()
	g.Approver = a
	g.mu.Unlock()
}

// currentApprover reads the approver under the lock.
func (g *Gate) currentApprover() Approver {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.Approver
}

// Check implements the tools registry's gate.
func (g *Gate) Check(ctx context.Context, toolName string, args json.RawMessage) (bool, string) {
	if g.Policy == nil {
		return true, ""
	}

	decision := g.Policy.Check(toolName, args)
	switch decision {
	case Allow:
		g.report(toolName, decision, "")
		return true, ""

	case Deny:
		reason := fmt.Sprintf("%s is not permitted", Describe(toolName, args))
		g.report(toolName, decision, reason)
		return false, reason
	}

	// Ask. The caller's own reason is part of the request: an approval shown
	// without one is a decision about a command rather than about an intention.
	request := Describe(toolName, args)
	if why := stringField(args, "why"); why != "" {
		request = request + "\n\nbecause: " + why
	}

	// Already answered. The person said no to this exact thing in this run, so
	// it is not put to them again — the model is told the answer stands, which
	// is the only way it learns not to keep knocking.
	if g.alreadyDeclined(request) {
		reason := fmt.Sprintf("%s was declined by the person you asked a moment ago. Do not repeat it — ask for something else if you need to.", request)
		g.report(toolName, decision, reason)
		return false, reason
	}

	approver := g.currentApprover()
	if approver == nil {
		reason := fmt.Sprintf("%s needs approval, and there is nobody to ask", request)
		g.report(toolName, decision, reason)
		return false, reason
	}

	switch approver.Approve(ctx, request) {
	case Approved:
		g.report(toolName, decision, "approved")
		return true, ""
	case Denied:
		g.noteDeclined(request)
		// It has to be clear that a person said no, and not that the harness is
		// broken or the tool is unavailable. A model that reads a refusal as a
		// wall stops asking and gives up in front of somebody who could have
		// approved something else a second later.
		reason := fmt.Sprintf("%s was declined by the person you asked — that is a no to this request, not the environment. Do not repeat it; ask for something else if you need to.", request)
		g.report(toolName, decision, reason)
		return false, reason
	default: // Unavailable
		reason := fmt.Sprintf("%s needs approval, and there is nobody to ask", request)
		g.report(toolName, decision, reason)
		return false, reason
	}
}

// alreadyDeclined reports whether this exact request has already been refused by
// the person in this run.
func (g *Gate) alreadyDeclined(request string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.declined[request]
}

// noteDeclined records that the person said no to this request.
func (g *Gate) noteDeclined(request string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.declined == nil {
		g.declined = map[string]bool{}
	}
	g.declined[request] = true
}

func (g *Gate) report(toolName string, decision Decision, reason string) {
	if g.OnDecision != nil {
		g.OnDecision(toolName, decision, reason)
	}
}
