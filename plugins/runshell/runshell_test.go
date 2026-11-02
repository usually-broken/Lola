package runshell

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/usually-broken/lola/core/tools"
)

func TestRunReturnsOutput(t *testing.T) {
	p := New(tools.New())
	got, err := p.Run(context.Background(), json.RawMessage(`{"command":"echo hello"}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(got) != "hello" {
		t.Fatalf("out = %q, want %q", got, "hello")
	}
}

func TestRunEmptyCommandIsAnError(t *testing.T) {
	p := New(tools.New())
	if _, err := p.Run(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected an error when command is missing")
	}
}

func TestRunReportsFailure(t *testing.T) {
	p := New(tools.New())
	if _, err := p.Run(context.Background(), json.RawMessage(`{"command":"exit 3"}`)); err == nil {
		t.Fatal("expected an error for a failing command")
	}
}
