// Package memory is the memory tool and its prompt part.
//
// Memory is a file: ~/.lola/memory.md, a run of entries separated by a line
// holding nothing but §. Adding an entry appends it, so nothing is rewritten
// and nothing already written can be lost to a bad edit elsewhere in the file.
//
// The file is read whenever the prompt is assembled, so what the model is told
// is whatever is on disk at the moment it is told.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/prompt"
	"github.com/usually-broken/lola/core/tools"
)

// Order puts memory after the persona: who this is, then what it knows.
const Order = 20

// Separator is the line between entries.
const Separator = "§"

// DefaultPath is the file itself.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".lola", "memory.md")
}

func init() {
	plugin.Register("memory", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{registry: d.Tools, prompts: d.Prompt, path: DefaultPath()}, nil
	})
}

// Plugin is the memory tool plus the prompt part that carries it.
type Plugin struct {
	registry *tools.Registry
	prompts  *prompt.Registry
	path     string
}

// New returns the plugin pointed at a file.
func New(reg *tools.Registry, prompts *prompt.Registry, path string) *Plugin {
	if path == "" {
		path = DefaultPath()
	}
	return &Plugin{registry: reg, prompts: prompts, path: path}
}

func (p *Plugin) Name() string { return "memory" }

func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindTool,
		What: "Memory: entries in ~/.lola/memory.md, separated by §, read into every prompt.",
	}
}

func (p *Plugin) Setup() (func() error, error) {
	if p.prompts != nil {
		if err := p.prompts.Add(prompt.Part{
			Name:  "memory",
			Order: Order,
			Live:  p.text,
			About: "What has been remembered. Appended by the memory tool, one entry at a time.",
		}); err != nil {
			return nil, err
		}
	}
	var teardown func() error = func() error { return nil }
	if p.registry != nil {
		var err error
		teardown, err = p.registry.Register(p)
		if err != nil {
			return nil, err
		}
	}

	// Unmounting has to take the prompt part with it, or switching memory off
	// would leave the text still being read into every prompt.
	return func() error {
		if p.prompts != nil {
			p.prompts.Remove("memory")
		}
		return teardown()
	}, nil
}

func (p *Plugin) Description() string {
	return "Remember something across sessions, or read back what is remembered. " +
		"Entries are kept in one file and put in front of you every turn. " +
		"action=read lists them with numbers, action=add writes a new one, " +
		"action=replace swaps an entry found by its number or by a phrase inside it."
}

func (p *Plugin) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["read", "add", "replace", "delete"], "description": "What to do."},
			"delete": {"type": "boolean", "description": "Set with action=replace to remove the entry instead of changing it."},
			"text":   {"type": "string", "description": "The entry to add, for action=add."},
			"index":  {"type": "integer", "description": "Entry number to replace, from action=read (1-based)."},
			"old":    {"type": "string", "description": "A phrase inside the entry to replace, when its number is not known."},
			"new":    {"type": "string", "description": "The complete replacement entry, for action=replace."}
		},
		"required": ["action"]
	}`)
}

type args struct {
	Action string `json:"action"`
	Text   string `json:"text"`
	Index  int    `json:"index"`
	Old    string `json:"old"`
	New    string `json:"new"`
	Delete bool   `json:"delete"`
}

// entries reads the file and splits it into entries.
func (p *Plugin) entries() []string {
	if p.path == "" {
		return nil
	}
	body, err := os.ReadFile(p.path)
	if err != nil {
		return nil
	}
	var out []string
	var current []string
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == Separator {
			if text := strings.TrimSpace(strings.Join(current, "\n")); text != "" {
				out = append(out, text)
			}
			current = nil
			continue
		}
		current = append(current, line)
	}
	if text := strings.TrimSpace(strings.Join(current, "\n")); text != "" {
		out = append(out, text)
	}
	return out
}

// text is the prompt part: every entry, in order.
func (p *Plugin) text() string {
	entries := p.entries()
	if len(entries) == 0 {
		return ""
	}
	return strings.Join(entries, "\n"+Separator+"\n")
}

func (p *Plugin) write(entries []string) error {
	if p.path == "" {
		return fmt.Errorf("memory: no path")
	}
	if dir := filepath.Dir(p.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("memory: create its directory: %w", err)
		}
	}
	body := strings.Join(entries, "\n"+Separator+"\n") + "\n"

	// Written beside the file and renamed over it, so a crash mid-write leaves
	// the old memory rather than half of the new one.
	temp := p.path + ".tmp"
	if err := os.WriteFile(temp, []byte(body), 0o644); err != nil {
		return fmt.Errorf("memory: %w", err)
	}
	return os.Rename(temp, p.path)
}

func (p *Plugin) Run(_ context.Context, raw json.RawMessage) (string, error) {
	var a args
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("memory: bad arguments: %w", err)
	}

	entries := p.entries()

	switch a.Action {
	case "read":
		if len(entries) == 0 {
			return "Memory is empty.", nil
		}
		var b strings.Builder
		for i, entry := range entries {
			fmt.Fprintf(&b, "%d. %s\n", i+1, entry)
		}
		return strings.TrimRight(b.String(), "\n"), nil

	case "add":
		text := strings.TrimSpace(a.Text)
		if text == "" {
			return "", fmt.Errorf("memory: nothing to remember")
		}
		entries = append(entries, text)
		if err := p.write(entries); err != nil {
			return "", err
		}
		return fmt.Sprintf("remembered (%d entries)", len(entries)), nil

	case "replace":
		replacement := strings.TrimSpace(a.New)
		if replacement == "" && !a.Delete {
			return "", fmt.Errorf("memory: no replacement text")
		}
		target := -1
		if a.Index > 0 && a.Index <= len(entries) {
			target = a.Index - 1
		} else if a.Old != "" {
			phrase := strings.ToLower(a.Old)
			for i, entry := range entries {
				if strings.Contains(strings.ToLower(entry), phrase) {
					target = i
					break
				}
			}
		}
		if target < 0 {
			return "", fmt.Errorf("memory: no entry matched; run read first")
		}
		if a.Delete {
			entries = append(entries[:target], entries[target+1:]...)
			if err := p.write(entries); err != nil {
				return "", err
			}
			return fmt.Sprintf("deleted entry %d (%d left)", target+1, len(entries)), nil
		}

		entries[target] = replacement
		if err := p.write(entries); err != nil {
			return "", err
		}
		return fmt.Sprintf("replaced entry %d of %d", target+1, len(entries)), nil

	default:
		return "", fmt.Errorf("memory: unknown action %q", a.Action)
	}
}

var _ tools.Tool = (*Plugin)(nil)
