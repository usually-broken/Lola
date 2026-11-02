// Package tui is the terminal frontend, as a plugin.
//
// It renders a full-screen interface: a LOLA header, a status bar showing the
// mounted plugins and the model, a scrollable transcript of the session, and a
// live view of what the agent is doing while it works.
//
// Unlike a plain output stream, the transcript is a viewport you can scroll
// with the arrow keys, Page Up/Down, or the mouse wheel, so earlier turns stay
// reachable however long the conversation gets.
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/agent"
	"github.com/usually-broken/lola/core/commands"
	"github.com/usually-broken/lola/core/frontend"
	"github.com/usually-broken/lola/core/guard"
	"github.com/usually-broken/lola/core/llm"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/session"
)

func init() {
	plugin.Register("tui", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{approver: newApprover(), commands: d.Commands}, nil
	})
}

// Plugin is the TUI frontend, mounted into the tree like any other plugin.
type Plugin struct {
	approver *approver
	commands *commands.Registry
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "tui" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindUI,
		What: "The terminal interface.",
	}
}

// Setup mounts the plugin.
func (p *Plugin) Setup() (func() error, error) { return func() error { return nil }, nil }

// Frontend returns the seam this plugin contributes: a terminal frontend.
func (p *Plugin) Frontend() frontend.Frontend {
	return &ui{approver: p.approver, commands: p.commands}
}

// Approver returns the consent seam this plugin contributes to the guard. The
// harness installs it, so gated tool calls ask the person in the interface.
func (p *Plugin) Approver() guard.Approver { return p.approver }

// Approver on the interface itself. The harness reaches the frontend, not the
// plugin, so the seam has to be visible from here too — without it the guard
// never asks and gated calls are just refused.
func (u *ui) Approver() guard.Approver { return u.approver }

// stepWatcher is implemented by agent loops that can report their steps live.
type stepWatcher interface {
	RunWatched(ctx context.Context, provider llm.Provider, model, task string, onStep func(llm.ToolCall)) (agent.Result, error)
}

// sessionAgent is implemented by agent loops that expose which session they are
// writing to. The conversation methods themselves (Reset, Resume) come from the
// agent.Agent seam.
type sessionAgent interface {
	SessionID() string
}

// --- styles ---------------------------------------------------------------

var (
	colAccent = lipgloss.Color("#2563eb")
	colLight  = lipgloss.Color("#93c5fd")
	colPale   = lipgloss.Color("#dbeafe")
	colDim    = lipgloss.Color("#64748b")
	colText   = lipgloss.Color("#e2e8f0")

	styleLogo = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#ffffff")).
			Background(colAccent).
			Padding(0, 2)

	styleBar   = lipgloss.NewStyle().Foreground(colDim)
	styleBarHi = lipgloss.NewStyle().Foreground(colLight)
	styleUser  = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	styleAgent = lipgloss.NewStyle().Foreground(colText)
	styleTool  = lipgloss.NewStyle().Foreground(colDim).Italic(true)
	styleErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444"))
	styleHint  = lipgloss.NewStyle().Foreground(colDim).Italic(true)
)

// --- messages -------------------------------------------------------------

type agentDoneMsg struct {
	res agent.Result
	err error
}

type stepMsg struct{ text string }

// approvalMsg carries a gated tool call's question from the guard to the UI.
type approvalMsg struct{ req approvalRequest }

// --- the model ------------------------------------------------------------

// picker is the session list overlay, shown when the user asks to resume or
// manage sessions.
type picker struct {
	open     bool
	sessions []*session.Session
	cursor   int
	mode     pickerMode
}

type pickerMode int

const (
	pickResume pickerMode = iota
	pickManage
)

type ui struct {
	rt frontend.Runtime

	width, height int
	ready         bool

	program  *tea.Program
	viewport viewport.Model
	content  strings.Builder
	input    textarea.Model
	inputOK  bool
	spin     spinner.Model

	store *session.Store
	pick  picker

	// approver carries a gated tool call's question to the interface; pending
	// is the question currently on screen, if any.
	approver *approver
	pending  *approvalRequest

	// commands is the registry of slash commands the interface offers.
	commands *commands.Registry

	// current is the id of the session being continued, empty for a new one.
	current string

	busy     bool
	activity string
}

func (u *ui) Run(_ context.Context, rt frontend.Runtime) error {
	u.rt = rt
	u.store = rt.Sessions

	u.spin = spinner.New()
	u.spin.Spinner = spinner.Dot
	u.spin.Style = lipgloss.NewStyle().Foreground(colAccent)

	u.viewport = viewport.New(80, 20)

	u.input = textarea.New()
	u.input.Placeholder = "Ask Lola to do something…  (/help for commands)"
	u.input.Prompt = ""
	u.input.CharLimit = 0
	u.input.ShowLineNumbers = false
	u.input.Focus()
	u.inputOK = true

	u.append(styleHint.Render("Type a task and press Enter. The agent will use its tools and report back."))
	u.append(styleHint.Render("Scroll with ↑/↓, Page Up/Down, or the mouse wheel. Tab focuses the transcript. Ctrl+C quits."))
	// Register the interface's own commands, so /help lists everything on
	// offer, including whatever a plugin contributed.
	u.registerCommands()
	u.append(styleHint.Render("Type /help to see the commands."))
	u.append("")

	u.program = tea.NewProgram(u, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := u.program.Run()
	return err
}

// Init is required by tea.Model. It arms the approval listener alongside the
// input blink, so a gated tool call can raise its question.
func (u *ui) Init() tea.Cmd {
	if u.approver == nil {
		return textarea.Blink
	}
	return tea.Batch(textarea.Blink, u.waitApproval())
}

// waitApproval blocks until the guard asks a question, then delivers it to the
// UI. It is re-armed after each question, so they keep arriving.
func (u *ui) waitApproval() tea.Cmd {
	return func() tea.Msg {
		if u.approver == nil {
			return nil
		}
		req, ok := <-u.approver.requests
		if !ok {
			return nil
		}
		return approvalMsg{req: req}
	}
}

// append adds a line to the transcript and keeps the viewport at the bottom.
func (u *ui) append(line string) {
	if u.content.Len() > 0 {
		u.content.WriteString("\n")
	}
	u.content.WriteString(line)
	u.viewport.SetContent(u.content.String())
	u.viewport.GotoBottom()
}

// Update handles input and messages.
func (u *ui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		u.width, u.height = msg.Width, msg.Height
		u.ready = true
		u.layout()
		return u, nil

	case tea.KeyMsg:
		// A pending approval owns the keyboard: y or n answers it.
		if u.pending != nil {
			return u.answerApproval(msg)
		}
		// The picker, when open, owns the keyboard.
		if u.pick.open {
			return u.updatePicker(msg)
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			return u, tea.Quit
		case tea.KeyEsc:
			return u, tea.Quit
		case tea.KeyTab:
			// Hand focus between the input and the transcript, so the same
			// arrow keys can either move the caret or scroll.
			if u.input.Focused() {
				u.input.Blur()
			} else {
				u.input.Focus()
			}
			return u, nil
		case tea.KeyEnter:
			if u.busy || !u.input.Focused() {
				break
			}
			task := strings.TrimSpace(u.input.Value())
			if task == "" {
				break
			}
			u.input.Reset()

			// A line starting with / is a command, not a task.
			if strings.HasPrefix(task, "/") {
				return u.runCommand(task)
			}

			u.append(styleUser.Render("› " + task))
			u.busy = true
			u.activity = "thinking…"
			return u, tea.Batch(u.spin.Tick, u.runAgent(task))
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		u.spin, cmd = u.spin.Update(msg)
		cmds = append(cmds, cmd)

	case approvalMsg:
		u.pending = &msg.req
		u.append(styleTool.Render("  ⚠ needs approval: " + msg.req.prompt))
		// Re-arm so the next question also arrives.
		cmds = append(cmds, u.waitApproval())
		return u, tea.Batch(cmds...)

	case stepMsg:
		u.activity = msg.text
		// Also write the tool call to the transcript, so the session keeps a
		// record of what the agent actually did, not just how many steps.
		u.append(styleTool.Render("  ↳ " + msg.text))

	case agentDoneMsg:
		u.busy = false
		u.activity = ""
		if msg.err != nil {
			u.append(styleErr.Render("error: " + msg.err.Error()))
			u.append("")
			return u, nil
		}
		for _, line := range strings.Split(strings.TrimSpace(msg.res.Content), "\n") {
			u.append(styleAgent.Render(line))
		}
		u.append(styleTool.Render(fmt.Sprintf("   (%d step%s)", msg.res.Steps, plural(msg.res.Steps))))
		u.append("")

		// The run just saved a session; remember which, so /pin and /archive
		// know what "the current session" means.
		if sa, ok := u.rt.Agent.(sessionAgent); ok {
			u.current = sa.SessionID()
		}
	}

	// The input handles its own keys when focused; the viewport handles
	// scrolling. Both always get a look, so the mouse wheel works regardless.
	var cmd tea.Cmd
	u.input, cmd = u.input.Update(msg)
	cmds = append(cmds, cmd)

	u.viewport, cmd = u.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return u, tea.Batch(cmds...)
}

// runCommand handles a /command line: it dispatches through the registry, so
// the commands are whatever has been registered — the interface's own, plus
// anything a plugin contributed — rather than a list fixed here.
func (u *ui) runCommand(line string) (tea.Model, tea.Cmd) {
	u.dispatch(line)
	return u, nil
}

// sessionLine renders one session for a list.
func (u *ui) sessionLine(s *session.Session) string {
	flags := ""
	if s.Pinned {
		flags += " ★"
	}
	if s.Archived {
		flags += " ⌗"
	}
	mark := "  "
	if s.ID == u.current {
		mark = "▸ "
	}
	return fmt.Sprintf("%s%s  %-44s %d msgs%s", mark, s.ID, truncate(s.Title, 44), len(s.Messages), flags)
}

// updatePicker handles keys while the session picker is open.
func (u *ui) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		u.pick.open = false
		return u, nil
	case tea.KeyUp:
		if u.pick.cursor > 0 {
			u.pick.cursor--
		}
		return u, nil
	case tea.KeyDown:
		if u.pick.cursor < len(u.pick.sessions)-1 {
			u.pick.cursor++
		}
		return u, nil
	case tea.KeyEnter:
		if len(u.pick.sessions) == 0 {
			u.pick.open = false
			return u, nil
		}
		sess := u.pick.sessions[u.pick.cursor]
		u.pick.open = false
		if err := u.rt.Agent.Resume(sess.ID); err != nil {
			u.append(styleErr.Render("could not resume " + sess.ID + ": " + err.Error()))
			return u, nil
		}
		u.current = sess.ID
		u.renderHistory()
		u.append(styleHint.Render("Resumed session " + sess.ID + " — " + sess.Title))
		u.append("")
		return u, nil
	}
	return u, nil
}

// renderHistory rebuilds the transcript from the resumed conversation.
func (u *ui) renderHistory() {
	u.content.Reset()
	u.viewport.SetContent("")
	for _, m := range u.rt.Agent.History() {
		switch m.Role {
		case llm.RoleUser:
			u.append(styleUser.Render("› " + m.Content))
		case llm.RoleAssistant:
			if strings.TrimSpace(m.Content) == "" {
				continue
			}
			for _, line := range strings.Split(strings.TrimSpace(m.Content), "\n") {
				u.append(styleAgent.Render(line))
			}
			u.append("")
		case llm.RoleTool:
			u.append(styleTool.Render("  ↳ " + compact(m.Content)))
		}
	}
}

// truncate shortens a string for a list column.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 1 {
		return ""
	}
	return s[:n-1] + "…"
}

// layout recomputes the pane sizes for the terminal.
func (u *ui) layout() {
	headerH := lipgloss.Height(u.header())
	statusH := lipgloss.Height(u.status())
	footerH := lipgloss.Height(u.footer())
	bodyH := u.height - headerH - statusH - footerH - 1
	if bodyH < 3 {
		bodyH = 3
	}
	u.viewport.Width = maxInt(20, u.width-4)
	u.viewport.Height = bodyH
	if u.inputOK {
		u.input.SetWidth(maxInt(20, u.width-6))
	}
	u.viewport.SetContent(u.content.String())
}

// runAgent runs one task off the UI goroutine. Each tool call is streamed back
// so the footer shows live activity.
func (u *ui) runAgent(task string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		report := func(call llm.ToolCall) {
			u.program.Send(stepMsg{text: fmt.Sprintf("calling %s %s", call.Name, compact(string(call.Arguments)))})
		}

		if watcher, ok := u.rt.Agent.(stepWatcher); ok {
			res, err := watcher.RunWatched(ctx, u.rt.Provider, u.rt.Model, task, report)
			return agentDoneMsg{res: res, err: err}
		}

		res, err := u.rt.Agent.Run(ctx, u.rt.Provider, u.rt.Model, task)
		return agentDoneMsg{res: res, err: err}
	}
}

// View renders the whole screen.
func (u *ui) View() string {
	if !u.ready {
		return "loading…"
	}
	if u.pick.open {
		return u.pickerView()
	}
	return lipgloss.JoinVertical(lipgloss.Left, u.header(), u.viewport.View(), u.status(), u.footer())
}

// pickerView renders the session list overlay.
func (u *ui) pickerView() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(colAccent).Render("Sessions")
	var lines []string
	lines = append(lines, title, "")
	for i, s := range u.pick.sessions {
		line := u.sessionLine(s)
		if i == u.pick.cursor {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(colAccent).Render(line)
		} else {
			line = lipgloss.NewStyle().Foreground(colLight).Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", styleHint.Render("↑/↓ to choose · Enter to resume · Esc to cancel"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Padding(1, 2).
		Width(u.width - 4).
		Height(u.height - 2).
		Render(strings.Join(lines, "\n"))
}

// header is the big LOLA banner plus the model line.
func (u *ui) header() string {
	logo := styleLogo.Render("L O L A")
	sub := lipgloss.NewStyle().Foreground(colPale).Render("  an agent harness — everything is a plugin")
	model := lipgloss.NewStyle().Foreground(colLight).Render("  model: " + u.rt.Model)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Padding(0, 1).
		Width(u.width - 4).
		Render(logo + "\n" + sub + "\n" + model)
}

// status shows the mounted plugins, the config, and how far down the transcript
// we are.
func (u *ui) status() string {
	plugins := styleBarHi.Render(strings.Join(u.rt.Plugins, " · "))
	line1 := styleBar.Render("plugins: ") + plugins
	line2 := styleBar.Render("config:  ") + styleBarHi.Render(u.rt.ConfigPath)
	line3 := styleBar.Render("sessions: ") + styleBarHi.Render(u.rt.SessionsDir) +
		styleBar.Render(fmt.Sprintf("   (%d%%)", int(u.viewport.ScrollPercent()*100)))
	return lipgloss.NewStyle().Padding(0, 1).Width(u.width - 2).Render(line1 + "\n" + line2 + "\n" + line3)
}

// footer shows a pending approval, the live activity, or the input box.
func (u *ui) footer() string {
	if u.pending != nil {
		return u.approvalFooter()
	}
	if u.busy {
		line := u.spin.View() + " " + lipgloss.NewStyle().Foreground(colLight).Render(u.activity)
		return lipgloss.NewStyle().Padding(0, 2).Width(u.width - 2).Render(line)
	}
	border := colDim
	if !u.input.Focused() {
		border = colAccent // focused transcript
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(u.width - 4).
		Render(u.input.View())
}

// approvalFooter renders the yes/no question for a gated tool call.
func (u *ui) approvalFooter() string {
	label := lipgloss.NewStyle().Foreground(colAccent).Bold(true).Render("approve? ")
	body := lipgloss.NewStyle().Foreground(colText).Render(u.pending.prompt)
	hint := styleHint.Render("   [y] yes   [n] no")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Padding(0, 1).
		Width(u.width - 4).
		Render(label + body + "\n" + hint)
}

// answerApproval handles a key while an approval is pending. y approves, n
// denies, anything else is ignored so a stray key cannot decide it.
func (u *ui) answerApproval(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		u.pending.reply <- guard.Approved
		u.append(styleHint.Render("  ⚠ approved: " + u.pending.prompt))
	case "n", "N":
		u.pending.reply <- guard.Denied
		u.append(styleHint.Render("  ⚠ denied: " + u.pending.prompt))
	default:
		return u, nil
	}
	u.pending = nil
	return u, nil
}

func compact(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 60 {
		return s[:57] + "…"
	}
	return s
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
