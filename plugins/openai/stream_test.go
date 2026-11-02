package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usually-broken/lola/core/llm"
)

// fakeStream is a server that answers with server-sent events, so the reader can
// be tested without a model, a network or a bill.
func fakeStream(t *testing.T, lines []string) *Provider {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, line := range lines {
			fmt.Fprint(w, line+"\n\n")
		}
	}))
	t.Cleanup(srv.Close)

	return New(Options{BaseURL: srv.URL})
}

func delta(content string) string {
	return fmt.Sprintf(`data: {"choices":[{"delta":{"content":%q}}]}`, content)
}

func TestStreamHandsBackTextAsItArrives(t *testing.T) {
	p := fakeStream(t, []string{
		delta("Hello"),
		delta(" there"),
		delta(", friend"),
		"data: [DONE]",
	})

	var got []string
	resp, err := p.Stream(context.Background(), llm.Request{Model: "m"},
		func(d string) error { got = append(got, d); return nil })
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	if strings.Join(got, "") != "Hello there, friend" {
		t.Errorf("deltas joined to %q", strings.Join(got, ""))
	}
	if len(got) != 3 {
		t.Errorf("got %d deltas, want 3 — text should arrive in pieces", len(got))
	}
	if resp.Content != "Hello there, friend" {
		t.Errorf("final content = %q", resp.Content)
	}
}

// A tool call arrives as fragments: the name once, then the arguments a few
// characters at a time. What comes back must be whole JSON.
func TestStreamReassemblesToolCalls(t *testing.T) {
	p := fakeStream(t, []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","function":{"name":"run_shell","arguments":"{\"comm"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"and\":\"date\""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]}}]}`,
		"data: [DONE]",
	})

	resp, err := p.Stream(context.Background(), llm.Request{Model: "m"}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(resp.ToolCalls))
	}
	call := resp.ToolCalls[0]
	if call.Name != "run_shell" || call.ID != "call-1" {
		t.Errorf("call = %+v", call)
	}
	if string(call.Arguments) != `{"command":"date"}` {
		t.Errorf("arguments = %s", call.Arguments)
	}
}

func TestStreamKeepsTwoCallsApart(t *testing.T) {
	p := fakeStream(t, []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"one","arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"b","function":{"name":"two","arguments":"{}"}}]}}]}`,
		"data: [DONE]",
	})

	resp, err := p.Stream(context.Background(), llm.Request{Model: "m"}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	if len(resp.ToolCalls) != 2 {
		t.Fatalf("got %d calls, want 2", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "one" || resp.ToolCalls[1].Name != "two" {
		t.Errorf("calls came back in the wrong order: %+v", resp.ToolCalls)
	}
}

// Text and a tool call in the same reply: the text is still handed over.
func TestStreamCarriesTextAndCallsTogether(t *testing.T) {
	p := fakeStream(t, []string{
		delta("I'll check the date."),
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c","function":{"name":"run_shell","arguments":"{\"command\":\"date\"}"}}]}}]}`,
		"data: [DONE]",
	})

	var said strings.Builder
	resp, err := p.Stream(context.Background(), llm.Request{Model: "m"},
		func(d string) error { said.WriteString(d); return nil })
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	if said.String() != "I'll check the date." {
		t.Errorf("said %q", said.String())
	}
	if len(resp.ToolCalls) != 1 {
		t.Errorf("lost the tool call: %+v", resp)
	}
}

// A server that cannot stream answers with a normal JSON error; that should be
// reported as the model's error, not as a parse failure.
func TestStreamReportsAnErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"model not found"}}`)
	}))
	t.Cleanup(srv.Close)

	p := New(Options{BaseURL: srv.URL})
	_, err := p.Stream(context.Background(), llm.Request{Model: "nope"}, nil)

	if err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("error = %v, want the model's message", err)
	}
}
