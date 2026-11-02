package tui

import (
	"fmt"
	"strings"

	"github.com/usually-broken/lola/core/commands"
)

// registerCommands puts the interface's own commands into the registry, so
// /help lists them alongside anything a plugin contributed. Nothing here is
// special-cased in the dispatcher: every command arrives the same way.
func (u *ui) registerCommands() {
	if u.commands == nil {
		u.commands = commands.New()
	}

	defs := []commands.Command{
		{
			Name:        "help",
			Description: "list the commands",
			Run: func([]string) (string, error) {
				var b strings.Builder
				b.WriteString("Commands:")
				for _, c := range u.commands.All() {
					usage := c.Usage
					if usage == "" {
						usage = "/" + c.Name
					}
					fmt.Fprintf(&b, "\n  %-22s %s", usage, c.Description)
				}
				return b.String(), nil
			},
		},
		{
			Name:        "new",
			Description: "start a new session",
			Run: func([]string) (string, error) {
				u.rt.Agent.Reset()
				u.current = ""
				u.content.Reset()
				u.viewport.SetContent("")
				return "Started a new session.", nil
			},
		},
		{
			Name:        "sessions",
			Description: "list saved sessions",
			Run: func([]string) (string, error) {
				list, err := u.store.List()
				if err != nil {
					return "", err
				}
				if len(list) == 0 {
					return "No saved sessions yet.", nil
				}
				var b strings.Builder
				for i, s := range list {
					if i > 0 {
						b.WriteString("\n")
					}
					b.WriteString(u.sessionLine(s))
				}
				return b.String(), nil
			},
		},
		{
			Name:        "resume",
			Description: "pick a session to continue",
			Run: func([]string) (string, error) {
				list, err := u.store.List()
				if err != nil {
					return "", err
				}
				if len(list) == 0 {
					return "No saved sessions to resume.", nil
				}
				u.pick = picker{open: true, sessions: list, mode: pickResume}
				return "", nil
			},
		},
		{
			Name:        "pin",
			Usage:       "/pin [id]",
			Description: "pin the current or named session",
			Run:         func(args []string) (string, error) { return u.flagSession(args, "pin") },
		},
		{
			Name:        "archive",
			Usage:       "/archive [id]",
			Description: "archive the current or named session",
			Run:         func(args []string) (string, error) { return u.flagSession(args, "archive") },
		},
		{
			Name:        "delete",
			Usage:       "/delete <id>",
			Description: "delete a session",
			Run: func(args []string) (string, error) {
				if len(args) == 0 {
					return "", fmt.Errorf("usage: /delete <id>")
				}
				if err := u.store.Delete(args[0]); err != nil {
					return "", err
				}
				return "Deleted session " + args[0] + ".", nil
			},
		},
		{
			Name:        "model",
			Usage:       "/model [name]",
			Description: "show or change the model",
			Run: func(args []string) (string, error) {
				if len(args) == 0 {
					return "model: " + u.rt.Model, nil
				}
				u.rt.Model = args[0]
				return "model is now " + args[0], nil
			},
		},
		{
			Name:        "workspace",
			Usage:       "/workspace [dir ...]",
			Description: "show or change where the agent may write",
			Run: func(args []string) (string, error) {
				if u.rt.Workspace == nil {
					return "no workspace is configured", nil
				}
				if len(args) == 0 {
					dirs := u.rt.Workspace.Dirs()
					if len(dirs) == 0 {
						return "no workspace is configured", nil
					}
					return "workspace:\n  " + strings.Join(dirs, "\n  "), nil
				}
				if err := u.rt.Workspace.Set(args...); err != nil {
					return "", err
				}
				return "workspace is now:\n  " + strings.Join(u.rt.Workspace.Dirs(), "\n  "), nil
			},
		},
		{
			Name:        "source",
			Usage:       "/source [dir]",
			Description: "show or change where the harness's own source lives",
			Run: func(args []string) (string, error) {
				if u.rt.Guard == nil {
					return "no guard is configured", nil
				}
				if len(args) == 0 {
					src := u.rt.Guard.SourceOf()
					if src == "" {
						return "no source is set, so editing your own code is not gated", nil
					}
					return "source: " + src, nil
				}
				u.rt.Guard.SetSource(args[0])
				return "source is now " + args[0], nil
			},
		},
	}

	for _, c := range defs {
		// A duplicate means something already claimed the name; keep theirs.
		_ = u.commands.Register(c)
	}
}

// flagSession pins or archives a session, named or the current one.
func (u *ui) flagSession(args []string, what string) (string, error) {
	id := u.current
	if len(args) > 0 {
		id = args[0]
	}
	if id == "" {
		return "", fmt.Errorf("no session yet — send a message first, or name one: /%s <id>", what)
	}

	var err error
	if what == "pin" {
		err = u.store.SetPinned(id, true)
	} else {
		err = u.store.SetArchived(id, true)
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Session %s %sed.", id, what), nil
}

// dispatch runs a typed /command through the registry and shows the result.
func (u *ui) dispatch(line string) {
	name, args := commands.Split(line)

	c, ok := u.commands.Get(name)
	if !ok {
		u.append(styleErr.Render("unknown command: /" + name + " (try /help)"))
		return
	}

	out, err := c.Run(args)
	if err != nil {
		u.append(styleErr.Render(err.Error()))
		return
	}
	for _, l := range strings.Split(out, "\n") {
		if l != "" {
			u.append(styleHint.Render(l))
		}
	}
}
