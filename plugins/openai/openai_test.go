package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usually-broken/lola/core/llm"
)

func TestCompleteParsesReply(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello there"}}]}`))
	}))
	defer srv.Close()

	p := New(Options{BaseURL: srv.URL, APIKey: "test-key"})

	resp, err := p.Complete(context.Background(), llm.Request{
		Model:    "gemma4:cloud",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "hello there" {
		t.Fatalf("content = %q, want %q", resp.Content, "hello there")
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q, want /chat/completions", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("auth = %q, want bearer token", gotAuth)
	}
	if gotBody["model"] != "gemma4:cloud" {
		t.Fatalf("model = %v, want gemma4:cloud", gotBody["model"])
	}
}

func TestCompleteOmitsAuthWhenNoKey(t *testing.T) {
	var hadAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hadAuth = r.Header.Get("Authorization") != ""
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	p := New(Options{BaseURL: srv.URL})
	if _, err := p.Complete(context.Background(), llm.Request{Model: "m"}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if hadAuth {
		t.Fatal("expected no Authorization header when no API key is set")
	}
}

func TestCompleteSurfacesProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"message":"model not found"}}`))
	}))
	defer srv.Close()

	p := New(Options{BaseURL: srv.URL})
	_, err := p.Complete(context.Background(), llm.Request{Model: "nope"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); got != "openai: model not found" {
		t.Fatalf("err = %q, want provider message", got)
	}
}

func TestCompleteRejectsEmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	p := New(Options{BaseURL: srv.URL})
	if _, err := p.Complete(context.Background(), llm.Request{Model: "m"}); err == nil {
		t.Fatal("expected an error for an empty choices array")
	}
}

// TestProviderSatisfiesSeam is the point of the package: the concrete adapter
// is usable anywhere an llm.Provider is, with no adapter-specific code.
func TestProviderSatisfiesSeam(t *testing.T) {
	var _ llm.Provider = New(Options{})
}
