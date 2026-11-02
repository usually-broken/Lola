package commands

import (
	"errors"
	"strings"
	"testing"
)

func cmd(name string) Command {
	return Command{
		Name:        name,
		Description: "a test command",
		Run:         func([]string) (string, error) { return "ran " + name, nil },
	}
}

func TestRegisterAndGet(t *testing.T) {
	r := New()
	if err := r.Register(cmd("new")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	for _, name := range []string{"new", "/new", "NEW", " /New "} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("Get(%q) should find the command", name)
		}
	}
}

func TestRunIsCalled(t *testing.T) {
	r := New()
	var got []string
	if err := r.Register(Command{
		Name: "echo",
		Run:  func(args []string) (string, error) { got = args; return "ok", nil },
	}); err != nil {
		t.Fatal(err)
	}

	c, _ := r.Get("echo")
	out, err := c.Run([]string{"a", "b"})
	if err != nil || out != "ok" {
		t.Fatalf("Run = %q, %v", out, err)
	}
	if len(got) != 2 || got[0] != "a" {
		t.Fatalf("args = %v", got)
	}
}

func TestDuplicateIsRejected(t *testing.T) {
	r := New()
	if err := r.Register(cmd("new")); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(cmd("new")); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("err = %v, want ErrDuplicate", err)
	}
}

func TestNamelessCommandIsRejected(t *testing.T) {
	r := New()
	if err := r.Register(Command{Run: func([]string) (string, error) { return "", nil }}); err == nil {
		t.Fatal("a command with no name should be rejected")
	}
}

func TestRunlessCommandIsRejected(t *testing.T) {
	r := New()
	if err := r.Register(Command{Name: "x"}); err == nil {
		t.Fatal("a command with no Run should be rejected")
	}
}

func TestAllIsSorted(t *testing.T) {
	r := New()
	for _, n := range []string{"zeta", "alpha", "mid"} {
		if err := r.Register(cmd(n)); err != nil {
			t.Fatal(err)
		}
	}

	all := r.All()
	if len(all) != 3 {
		t.Fatalf("All() returned %d, want 3", len(all))
	}
	if all[0].Name != "alpha" || all[2].Name != "zeta" {
		t.Fatalf("All() = %v, want sorted", []string{all[0].Name, all[1].Name, all[2].Name})
	}
}

func TestSplit(t *testing.T) {
	name, args := Split("/workspace ~/somewhere --flag")
	if name != "workspace" {
		t.Errorf("name = %q", name)
	}
	if len(args) != 2 || args[0] != "~/somewhere" {
		t.Errorf("args = %v", args)
	}

	name, args = Split("")
	if name != "" || args != nil {
		t.Errorf("Split(\"\") = %q, %v", name, args)
	}
}

func TestNormalise(t *testing.T) {
	for in, want := range map[string]string{
		"/new":  "new",
		"new":   "new",
		" NEW ": "new",
		"/":     "",
	} {
		if got := Normalise(in); got != want {
			t.Errorf("Normalise(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConcurrentRegistrationIsSafe(t *testing.T) {
	r := New()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			_ = r.Register(cmd(strings.Repeat("x", i+1)))
		}
	}()
	for i := 0; i < 50; i++ {
		_ = r.All()
	}
	<-done
}
