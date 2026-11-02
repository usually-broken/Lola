package agentloop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/usually-broken/lola/core/agent"
	"github.com/usually-broken/lola/core/llm"
	"github.com/usually-broken/lola/core/plugin"
	"github.com/usually-broken/lola/core/tools"
)

// scriptedProvider replays a fixed list of responses, one per Complete call.
type scriptedProvider struct {
	responses []llm.Response
	calls     int
	lastReq   llm.Request
	repeat    bool
}

func (s *scriptedProvider) Name() string { return "scripted" }

func (s *scriptedProvider) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	s.lastReq = req
	if s.calls >= len(s.responses) {
		if s.repeat && len(s.responses) > 0 {
			return s.responses[len(s.responses)-1], nil
		}
		return llm.Response{Content: "done"}, nil
	}
	resp := s.responses[s.calls]
	s.calls++
	return resp, nil
}

type echoTool struct{ out string }

func (e *echoTool) Name() string            { return "echo" }
func (e *echoTool) Description() string     { return "echoes" }
func (e *echoTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (e *echoTool) Run(context.Context, json.RawMessage) (string, error) {
	return e.out, nil
}

func newLoop(t *testing.T, tools *tools.Registry) *Plugin {
	t.Helper()
	p, err := plugin.Build("agent_loop", plugin.Deps{Tools: tools})
	if err != nil {
		t.Fatalf("build agent_loop: %v", err)
	}
	return p.(*Plugin)
}

func TestRunFinishesWhenModelAnswersWithoutTool(t *testing.T) {
	p := &scriptedProvider{responses: []llm.Response{{Content: "all done"}}}
	loop := newLoop(t, tools.New())

	res, err := loop.Run(context.Background(), p, "m", "do nothing")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Content != "all done" || res.Steps != 1 {
		t.Fatalf("got %q in %d steps, want %q in 1", res.Content, res.Steps, "all done")
	}
}

func TestRunExecutesToolThenFinishes(t *testing.T) {
	p := &scriptedProvider{responses: []llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "echo", Arguments: json.RawMessage(`{"why":"testing the loop"}`)}}},
		{Content: "finished"},
	}}
	reg := tools.New()
	if _, err := reg.Register(&echoTool{out: "tool ran"}); err != nil {
		t.Fatal(err)
	}
	loop := newLoop(t, reg)

	res, err := loop.Run(context.Background(), p, "m", "use the tool")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Steps != 2 {
		t.Fatalf("steps = %d, want 2", res.Steps)
	}
	var found bool
	for _, m := range res.Messages {
		if m.Role == llm.RoleTool && m.ToolCallID == "call-1" && m.Content == "tool ran" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tool result not in conversation: %+v", res.Messages)
	}
}

func TestRunAdvertisesRegisteredTools(t *testing.T) {
	p := &scriptedProvider{responses: []llm.Response{{Content: "ok"}}}
	reg := tools.New()
	if _, err := reg.Register(&echoTool{}); err != nil {
		t.Fatal(err)
	}
	loop := newLoop(t, reg)

	if _, err := loop.Run(context.Background(), p, "m", "hi"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(p.lastReq.Tools) != 1 || p.lastReq.Tools[0].Name != "echo" {
		t.Fatalf("tools advertised = %+v, want [echo]", p.lastReq.Tools)
	}
}

func TestRunReportsToolErrorsToTheModel(t *testing.T) {
	p := &scriptedProvider{responses: []llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c", Name: "missing", Arguments: json.RawMessage(`{"why":"testing an unknown tool"}`)}}},
		{Content: "recovered"},
	}}
	loop := newLoop(t, tools.New())

	res, err := loop.Run(context.Background(), p, "m", "call a tool that does not exist")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var reported bool
	for _, m := range res.Messages {
		if m.Role == llm.RoleTool && strings.HasPrefix(m.Content, "error:") {
			reported = true
		}
	}
	if !reported {
		t.Fatalf("tool error not reported: %+v", res.Messages)
	}
}

func TestRunStopsAtMaxSteps(t *testing.T) {
	p := &scriptedProvider{
		responses: []llm.Response{
			{ToolCalls: []llm.ToolCall{{ID: "c", Name: "echo", Arguments: json.RawMessage(`{"why":"testing the loop"}`)}}},
		},
		repeat: true,
	}
	reg := tools.New()
	if _, err := reg.Register(&echoTool{}); err != nil {
		t.Fatal(err)
	}
	loop := newLoop(t, reg)
	loop.maxSteps = 3

	_, err := loop.Run(context.Background(), p, "m", "loop forever")
	if !errors.Is(err, agent.ErrMaxSteps) {
		t.Fatalf("err = %v, want ErrMaxSteps", err)
	}
}

func TestLoopSatisfiesSeam(t *testing.T) {
	var _ agent.Agent = newLoop(t, tools.New())
}

// --- conversation history -------------------------------------------------

func TestFollowUpSeesEarlierTurns(t *testing.T) {
	p := &scriptedProvider{responses: []llm.Response{
		{Content: "first answer"},
		{Content: "second answer"},
	}}
	loop := newLoop(t, tools.New())

	if _, err := loop.Run(context.Background(), p, "m", "first task"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// The second run must send the whole history, not just the new task.
	p.calls = 0
	if _, err := loop.Run(context.Background(), p, "m", "second task"); err != nil {
		t.Fatalf("second run: %v", err)
	}

	msgs := p.lastReq.Messages
	if len(msgs) < 3 {
		t.Fatalf("second run sent %d messages, want the earlier turns included", len(msgs))
	}
	// The conversation opens with the instruction, then the earlier task.
	if msgs[0].Role != llm.RoleSystem {
		t.Fatalf("first message is %q, want the system instruction", msgs[0].Role)
	}
	if msgs[1].Content != "first task" {
		t.Fatalf("second message = %q, want the first task", msgs[1].Content)
	}
	if msgs[len(msgs)-1].Content != "second task" {
		t.Fatalf("last message = %q, want the second task", msgs[len(msgs)-1].Content)
	}
}

func TestHistoryGrowsAcrossRuns(t *testing.T) {
	p := &scriptedProvider{responses: []llm.Response{
		{Content: "a"}, {Content: "b"}, {Content: "c"},
	}}
	loop := newLoop(t, tools.New())

	for _, task := range []string{"one", "two", "three"} {
		if _, err := loop.Run(context.Background(), p, "m", task); err != nil {
			t.Fatalf("run %q: %v", task, err)
		}
	}

	// one system instruction, three user turns, three assistant turns
	if got := loop.conv.Len(); got != 7 {
		t.Fatalf("history holds %d messages, want 7", got)
	}
}

func TestResetClearsHistory(t *testing.T) {
	p := &scriptedProvider{responses: []llm.Response{{Content: "a"}}}
	loop := newLoop(t, tools.New())

	if _, err := loop.Run(context.Background(), p, "m", "something"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if loop.conv.Len() == 0 {
		t.Fatal("history should not be empty after a run")
	}

	loop.Reset()
	if loop.conv.Len() != 0 {
		t.Fatalf("history holds %d messages after Reset, want 0", loop.conv.Len())
	}
	if len(loop.History()) != 0 {
		t.Fatal("History() should be empty after Reset")
	}
}

func TestHistoryIncludesToolCalls(t *testing.T) {
	p := &scriptedProvider{responses: []llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "echo", Arguments: json.RawMessage(`{"why":"testing the loop"}`)}}},
		{Content: "done"},
	}}
	reg := tools.New()
	if _, err := reg.Register(&echoTool{out: "ran"}); err != nil {
		t.Fatal(err)
	}
	loop := newLoop(t, reg)

	if _, err := loop.Run(context.Background(), p, "m", "use it"); err != nil {
		t.Fatalf("run: %v", err)
	}

	var hasToolCall, hasToolResult bool
	for _, m := range loop.History() {
		if len(m.ToolCalls) > 0 {
			hasToolCall = true
		}
		if m.Role == llm.RoleTool && m.ToolCallID == "c1" {
			hasToolResult = true
		}
	}
	if !hasToolCall || !hasToolResult {
		t.Fatalf("history should keep tool calls and results: %+v", loop.History())
	}
}
