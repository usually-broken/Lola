// Package agentloop is the default agent loop, as a plugin.
//
// It drives one task to completion: send the conversation to the model, run
// whatever tools it asks for, feed the results back, and repeat until the model
// answers without calling a tool.
//
// The conversation persists across runs, so a follow-up task is a continuation
// rather than a fresh start. Reset starts over.
package agentloop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/usually-broken/lola/core"
	"github.com/usually-broken/lola/core/agent"
	"github.com/usually-broken/lola/core/llm"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/prompt"
	"github.com/usually-broken/lola/core/session"
	"github.com/usually-broken/lola/core/tools"
)

// DefaultMaxSteps bounds one Run.
const DefaultMaxSteps = 20

func init() {
	plugin.Register("agent_loop", func(d plugin.Deps) (core.Plugin, error) {
		return &Plugin{tools: d.Tools, plugins: d.Plugins, prompt: d.Prompt}, nil
	})
}

// Plugin is the agent loop, mounted into the tree like any other plugin.
type Plugin struct {
	tools    *tools.Registry
	maxSteps int
	conv     agent.Conversation

	// prompt is what the model is told first, assembled by whoever is mounted.
	// Nil means nobody is, and the text below is used instead.
	prompt *prompt.Registry

	// plugins is the tree, asked for a session store when one is needed. The
	// loop does not build its own: whoever owns the sessions folder offers one.
	plugins plugin.Plugins

	// store persists the conversation, once it has been found. Nil until then,
	// and stays nil if nothing in the tree keeps sessions — in which case this
	// loop keeps its memory in memory, which is not an error.
	store *session.Store

	// sess is the session being written to, created on the first save.
	sess *session.Session

	// OnStep, if set, is called after each model call with that step's
	// assistant message, so a caller can watch the loop work.
	OnStep func(step int, msg llm.Message)

	// OnSay is called with anything the model says during a run.
	OnSay func(text string)

	// OnDelta is called with each piece of text as it arrives, when the model
	// adapter can stream. OnSay is for a whole sentence before a tool call;
	// this is for the sentence being written.
	OnDelta func(delta string)
}

// Name identifies this plugin.
func (p *Plugin) Name() string { return "agent_loop" }

// About says what this is, so the plugins screen can group it and say what
// it is for rather than only what it is called.
func (p *Plugin) About() plugin.About {
	return plugin.About{
		Kind:  plugin.KindCore,
		What:  "The loop: takes a task, calls the model, runs what it asks for, stops.",
		Vital: true,
	}
}

// Setup mounts the plugin. The loop needs nothing set up.
func (p *Plugin) Setup() (func() error, error) { return func() error { return nil }, nil }

// toolDefs builds the tool list the model is shown from the registry.
func (p *Plugin) toolDefs() []llm.ToolDef {
	names := p.tools.Names()
	defs := make([]llm.ToolDef, 0, len(names))
	for _, name := range names {
		t, ok := p.tools.Get(name)
		if !ok {
			continue
		}
		defs = append(defs, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			// Every call carries a reason, so every schema asks for one. Done
			// here rather than in each plugin: a tool written later should not be
			// able to forget.
			Schema: tools.WithWhy(t.Schema()),
		})
	}
	return defs
}

// History returns the conversation so far.
func (p *Plugin) History() []llm.Message { return p.conv.Messages() }

// Reset starts a new conversation. The previous session stays on disk.
func (p *Plugin) Reset() {
	p.conv.Reset()
	p.sess = nil
}

// SessionID returns the ID of the session being written, empty until the first
// save. Useful for a frontend that shows which session is live.
func (p *Plugin) SessionID() string {
	if p.sess == nil {
		return ""
	}
	return p.sess.ID
}

// sessionStore returns the store to keep sessions in: whatever the tree offers,
// asked for once and remembered. Nothing offering one means this loop does not
// persist, which is the honest reading of a config with no sessions plugin.
func (p *Plugin) sessionStore() *session.Store {
	if p.store != nil {
		return p.store
	}
	if keeper, ok := plugin.Find[session.Keeper](p.plugins); ok {
		p.store = keeper.Sessions()
	}
	return p.store
}

// Resume loads a saved session into this loop, so the conversation continues
// where it left off. It replaces whatever is currently in memory.
func (p *Plugin) Resume(id string) error {
	store := p.sessionStore()
	if store == nil {
		return fmt.Errorf("agent: nothing in the tree keeps sessions")
	}
	sess, err := store.Load(id)
	if err != nil {
		return err
	}
	p.sess = sess
	p.conv.Reset()
	p.conv.Append(sess.Messages...)
	return nil
}

// persist writes the current conversation to the session store, creating the
// session on first use. A loop with no store is not persisted, and that is not
// an error — it just means this loop keeps its memory in memory.
func (p *Plugin) persist(title string) {
	store := p.sessionStore()
	if store == nil {
		return
	}
	if p.sess == nil {
		p.sess = &session.Session{}
	}
	p.sess.Messages = p.conv.Messages()
	if p.sess.Title == "" {
		p.sess.Title = title
	}
	// A failed save must not lose the conversation in memory, so the error is
	// surfaced to the caller only via the session id staying empty. Saving is
	// best-effort by design.
	_ = store.Save(p.sess)
}

// systemPrompt is what the model is told before anything else.
//
// It exists because there was nothing: no instruction about how to talk, and none
// about the one habit that makes a long run bearable — saying what you are about
// to do before you do it. The reason on each call covers approvals; this covers
// the person watching.
const systemPrompt = `You are the agent inside this harness, working for one person who is watching.

Before you call a tool, say a short line in your own words about what you are about to do and why. Not a heading, not a plan — a sentence, the way you would say it aloud. Then make the call.

After a tool returns, say no more than is needed: often nothing, or one line. Do not summarise what the person just watched happen.

Be brief and concrete. Do not pad, do not restate the question, and do not explain what you are about to explain.`

// Run drives one task to completion.
//
// The task is appended to the conversation, so the model sees everything said
// before it. If the model answers with text and no tool call, that is the
// result. If it asks for tools, each is run and its output appended as a tool
// message, and the loop goes round again — up to maxSteps times, after which
// agent.ErrMaxSteps is returned.
func (p *Plugin) Run(ctx context.Context, provider llm.Provider, model string, task string) (agent.Result, error) {
	return p.run(ctx, provider, model, task, nil)
}

// RunWatched is Run, but calls onStep for each tool call as it happens, so a
// frontend can show live activity.
func (p *Plugin) RunWatched(ctx context.Context, provider llm.Provider, model, task string, onStep func(llm.ToolCall)) (agent.Result, error) {
	return p.run(ctx, provider, model, task, onStep)
}

func (p *Plugin) run(ctx context.Context, provider llm.Provider, model, task string, onStep func(llm.ToolCall)) (agent.Result, error) {
	// At the front, once: a resumed session must not collect copies of the
	// instruction. The text is asked for each run rather than remembered, so a
	// plugin mounted mid-conversation is in the very next prompt — and one that
	// is unmounted stops being in it.
	instruction := systemPrompt
	if p.prompt != nil {
		if assembled := p.prompt.Text(); assembled != "" {
			instruction = assembled
		}
	}

	existing := p.conv.Messages()
	if len(existing) > 0 && existing[0].Role == llm.RoleSystem {
		existing[0].Content = instruction
	} else {
		p.conv.Append(llm.Message{Role: llm.RoleSystem, Content: instruction})
	}

	maxSteps := p.maxSteps
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}

	// Append the new task to the conversation rather than starting fresh.
	p.conv.Append(llm.Message{Role: llm.RoleUser, Content: task})
	defs := p.toolDefs()

	for step := 1; step <= maxSteps; step++ {
		req := llm.Request{
			Model:    model,
			Messages: p.conv.Messages(),
			Tools:    defs,
		}

		// Stream when the adapter can, so text reaches the person as it is
		// written rather than all at once at the end.
		var (
			resp llm.Response
			err  error
		)
		streamed := false
		if s, ok := provider.(llm.Streamer); ok {
			resp, err = s.Stream(ctx, req, func(delta string) error {
				if delta == "" {
					return nil
				}
				streamed = true
				if p.OnDelta != nil {
					p.OnDelta(delta)
				}
				return nil
			})
		} else {
			resp, err = provider.Complete(ctx, req)
		}
		if err != nil {
			return agent.Result{}, fmt.Errorf("agent: step %d: %w", step, err)
		}

		assistant := llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		p.conv.Append(assistant)

		if p.OnStep != nil {
			p.OnStep(step, assistant)
		}

		// Anything it said on the way to asking for a tool is a message in its
		// own right. The final message is not reported here: it is the answer,
		// and reporting it as well would draw it twice.
		if resp.Content != "" && len(resp.ToolCalls) > 0 && p.OnSay != nil && !streamed {
			p.OnSay(resp.Content)
		}

		// No tool calls means the model is done.
		if len(resp.ToolCalls) == 0 {
			p.persist(title(task))
			return agent.Result{Content: resp.Content, Messages: p.conv.Messages(), Steps: step}, nil
		}

		// Run each requested tool and feed the results back.
		for _, call := range resp.ToolCalls {
			if onStep != nil {
				onStep(call)
			}
			p.conv.Append(p.runTool(ctx, call))
		}
	}

	p.persist(title(task))
	return agent.Result{Messages: p.conv.Messages(), Steps: maxSteps}, agent.ErrMaxSteps
}

// title makes a short label from a task, for a session list.
func title(task string) string {
	t := strings.TrimSpace(strings.ReplaceAll(task, "\n", " "))
	if len(t) > 60 {
		return t[:57] + "…"
	}
	return t
}

// runTool runs one tool call and wraps its result as a tool message. A tool
// error is reported to the model as text rather than aborting the run, so the
// model can see what went wrong and try something else.
func (p *Plugin) runTool(ctx context.Context, call llm.ToolCall) llm.Message {
	args := call.Arguments
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}

	out, err := p.tools.Run(ctx, call.Name, args)
	if err != nil {
		out = "error: " + err.Error()
	}
	return llm.Message{
		Role:       llm.RoleTool,
		Content:    out,
		ToolCallID: call.ID,
	}
}

// Watch registers a function called with each step's assistant message, so a
// caller can watch the loop work without knowing what kind of plugin this is.
func (p *Plugin) Watch(onStep agent.StepFunc) {
	p.OnStep = onStep
}

// WatchDelta registers a function called with text as it is generated.
func (p *Plugin) WatchDelta(onDelta func(string)) {
	p.OnDelta = onDelta
}

// WatchSay registers a function called with anything the model says mid-run.
//
// Without it a run is silent until it finishes, which makes a long run look like
// nothing happening — and in voice it means dead air instead of "I'll run the
// date command".
func (p *Plugin) WatchSay(onSay func(string)) {
	p.OnSay = onSay
}

// Assert at compile time that the loop satisfies the seams.
var (
	_ agent.Agent   = (*Plugin)(nil)
	_ agent.Watcher = (*Plugin)(nil)
)
