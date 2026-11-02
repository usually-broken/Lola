# Writing a plugin

A plugin is the only kind of thing this harness is made of. The loop, the tools,
the model connection, the page, each settings section — all of them are plugins,
and none of them is special.

This is how to write one.

## The shape

Two methods, and a factory line:

```go
package greet

import (
	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/plugin"
)

func init() {
	plugin.Register("greet", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{}, nil
	})
}

type Plugin struct{}

func (p *Plugin) Name() string { return "greet" }

// Setup runs when the plugin is mounted. Whatever it registers, it gives back
// in the teardown.
func (p *Plugin) Setup() (func() error, error) {
	return func() error { return nil }, nil
}
```

`core.Plugin` is those two methods: `Name()` and `Setup()`. Everything else is
optional, and opt-in by implementing another interface.

## Getting into the binary

Registration happens in `init`, which only runs if something imports the package.
That something is `catalog/all.go`:

```go
_ "github.com/usually-broken/lola/plugins/greet"
```

Add a line there and the plugin exists. Whether it *mounts* is decided by the
config, not by the catalogue:

```yaml
plugins:
  - greet
```

So there are three states: in the catalogue and not configured (available), in
the catalogue and configured (mounted at start), and mounted at runtime by
switching it on in the Plugins screen. Only the last two do anything.

## What you are handed

`plugin.Deps` is the seam. Every field is an interface or a registry, never a
concrete package, so a plugin depends on what a thing does rather than where it
lives.

| Field | What it is for |
|---|---|
| `Tools` | offering something the model can call |
| `Commands` | offering a slash command |
| `Panes` | offering a panel to a frontend |
| `Contributions` | offering a chip or a control in a named place |
| `Routes` | offering an HTTP path |
| `Plugins` | asking what is mounted, and mounting or unmounting |
| `Modes` | the mode in force, and what modes exist |
| `Consent` | how much the harness asks before it acts |
| `Sessions` | the saved conversations |
| `Settings` | what the harness is set to |
| `Workspace` | the folders a run may touch |

Any of them can be nil. A nil seam is not a failure — it means nothing is
offering that thing, and a plugin should do nothing rather than panic. That rule
is not decoration: a page that assumes its controls exist breaks the moment a
section is switched off.

## Saying what you are

Implement `plugin.Described` and the Plugins screen can group you and say what
you are for:

```go
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind: plugin.KindTool,
		What: "Reads a file from the workspace.",
	}
}
```

Kinds are `KindUI`, `KindChip`, `KindTool`, `KindKeeper`, `KindModel`,
`KindPrompt`, `KindVoice`, `KindCore`, plus anything else you care to write, which
is shown as its own group.

The kind is not decoration: it decides where you appear in the plugins list, and
it is how a person finds you. A prompt part is not a tool the model calls and not
a chip you press, so it gets `KindPrompt`; a mouth, an ear or a mode gets
`KindVoice` because that is the thing they belong to, not the shape of their
interface. `Vital: true` is advice that this one probably should stay — advice only,
because a harness that will not be rearranged is not one this project is about.

Describing is optional. Undescribed plugins are grouped as "Not described".

## Offering something

### A tool the model can call

Every tool call carries a reason, and the registry refuses one that does not. The
reason is not for you — it is the sentence the approval card shows, and the thing
that stops the model doing something unannounced. The schema asks for it on every
tool whoever builds the list, so a plugin cannot opt out by forgetting.

Register it in `Setup`, and return a teardown that removes it:

```go
d.Tools.Add(tools.Tool{
	Name: "read_file",
	Run:  func(ctx context.Context, args map[string]any) (string, error) { ... },
})
```

Tools go through the guard. A path-carrying tool is refused, or asks, depending
on the mode — that is the harness's decision, not yours.

### A path

```go
p.routes.Handle("/api/thing", p.handle)
```

The path is taken back in the teardown with `p.routes.Remove("/api/thing")`, so a
plugin switched off in a running tree stops answering. If it does not, a switch
looks like it does nothing.

### A panel

This is how the settings sections are built, and there are two patterns.

**Describe your fields and let the panel draw them.** The panel uses its own
markup, so your section looks like the panel it is in:

```go
p.panes.Add(plugin.Pane{
	Slot:  "settings",
	Title: "General",
	Order: 10,
	Fields: []plugin.Field{
		{Key: "model", Label: "Model", Why: "Which model tasks run against.", Kind: "select"},
		{Key: "workspace", Label: "Workspace", Why: "Folders the file tools may touch.", Kind: "list"},
	},
})
```

`Kind` is `text`, `select` or `list`. Saving goes through the settings seam; you
do not write a save button.

**Or supply the markup yourself**, when the section is more than fields:

```go
p.panes.Add(plugin.Pane{Slot: "settings", Title: "Permissions", Order: 20, Body: body})
```

The markup is drawn inside the pane, so the classes are the frontend's and it
inherits the panel's styling. Use the panel's classes — `.field`, `.label`,
`.why`, `.rows`, `.row`, `.intro`, `.group-heading` — not your own inline sizes,
or your section will look like a different program. That mistake has been made
here; it looked exactly as bad as it sounds.

**Or render it live**, when the content changes while the panel is open:

```go
p.panes.Add(plugin.Pane{Slot: "settings", Title: "Plugins", Order: 30, Source: "/api/tree/html"})
```

The frontend fetches that path each time it draws. The tree plugin does this,
because only it knows what is mounted.

`Order` is where you sit among panes in the same slot. Lower comes first.

### A chip

```go
p.out.Add(plugin.Contribution{
	Place: plugin.PlaceAboveComposer,
	Kind:  plugin.KindSelect,
	Name:  "mode",
	Label: "Mode",
})
```

A contribution may also carry a `Script`. The page loads it once, when the
fragment is first drawn, because markup inserted into a page is inert: a picture
with behaviour needs code, and the code belongs to the plugin that drew it. That
is how the voice ball animates without the page knowing what a microphone is.

Places are named by the frontend. If the frontend has nowhere to put your
contribution, it is dropped and reported — it will not be drawn in a random spot.

## Remembering something

Implement `plugin.Keeper` and you can keep values between runs. Implement
`plugin.Sessions` and the conversations come through you. Both are how this
harness avoids growing a database in its core.

Keys are never kept by the harness itself: a provider entry names the environment
variable its key comes from, and the key is read at the moment of the call. Do
the same in anything you write.

## Being the model

`plugin.Router` is where calls go. `llmrouter` implements it, and the loop asks
the tree for it rather than for a package. That is why adding a provider *kind* —
Anthropic, say, with its own auth and body shape — is a plugin and not a change
to the core.

## Rules that came out of doing it wrong

Mounting changes the running tree only. Never write to the config file from a
plugin; what is switched off comes back on the next start, and that is deliberate.

Take back everything you offered. Panes, routes, tools, contributions — a
registry that only grows is a switch that appears not to work.

Do nothing when a seam is nil. Half this harness's bugs have been code assuming a
control it drew is still on the page.

Describe rather than draw, when a frontend already knows how. Your markup will
not match, and matching is most of looking right.

One feature per pull request, with tests, and `gofmt`/`go vet`/`go test` clean
before it goes up. There is no CI to catch you while the minutes are spent.
