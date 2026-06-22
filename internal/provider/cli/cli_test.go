package cli

import (
	"context"
	"strings"
	"testing"
)

// These tests use ubiquitous shell tools as stand-in "agent CLIs" so the
// stdin/argument plumbing is verified without invoking real (paid, side-effecting)
// agents.

func TestStdinMode(t *testing.T) {
	// `cat` echoes whatever it receives on stdin.
	p := New(Spec{Name: "fake", Bin: "cat", Stdin: true})
	out, err := p.Ask(context.Background(), "hello via stdin")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello via stdin" {
		t.Fatalf("stdin mode: got %q", out)
	}
}

func TestArgMode(t *testing.T) {
	// `echo` prints its arguments; the prompt is appended as the final arg.
	p := New(Spec{Name: "fake", Bin: "echo", Stdin: false})
	out, err := p.Ask(context.Background(), "hello via arg")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello via arg" {
		t.Fatalf("arg mode: got %q", out)
	}
}

func TestPlaceholderSubstitution(t *testing.T) {
	p := New(Spec{Name: "fake", Bin: "echo", Args: []string{"prefix", promptPlaceholder, "suffix"}})
	out, err := p.Ask(context.Background(), "MID")
	if err != nil {
		t.Fatal(err)
	}
	if out != "prefix MID suffix" {
		t.Fatalf("placeholder: got %q", out)
	}
}

func TestAvailability(t *testing.T) {
	if !New(Spec{Bin: "sh"}).Available() {
		t.Fatal("sh should be available")
	}
	if New(Spec{Bin: "definitely-not-a-real-binary-xyz"}).Available() {
		t.Fatal("nonexistent binary should be unavailable")
	}
}

func TestHealthRunnable(t *testing.T) {
	// `echo hi` exits 0 and prints "hi" — a healthy, runnable probe.
	p := New(Spec{Name: "fake", Bin: "echo", HealthArgs: []string{"hi"}})
	detail, err := p.Health(context.Background())
	if err != nil {
		t.Fatalf("expected healthy, got %v", err)
	}
	if detail != "hi" {
		t.Fatalf("detail = %q, want first probe line %q", detail, "hi")
	}
}

func TestHealthNotInstalled(t *testing.T) {
	p := New(Spec{Name: "fake", Bin: "definitely-not-a-real-binary-xyz"})
	if _, err := p.Health(context.Background()); err == nil {
		t.Fatal("expected error for a missing binary")
	} else if !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("error should say not installed: %v", err)
	}
}

func TestHealthNotRunnable(t *testing.T) {
	// `false` is on PATH but exits non-zero → installed but not runnable.
	p := New(Spec{Name: "fake", Bin: "false", HealthArgs: []string{"--version"}})
	if _, err := p.Health(context.Background()); err == nil {
		t.Fatal("expected error for a failing probe")
	} else if !strings.Contains(err.Error(), "not runnable") {
		t.Fatalf("error should say not runnable: %v", err)
	}
}

func TestBuiltinsDeclareVersionProbe(t *testing.T) {
	for _, s := range Builtins() {
		if len(s.HealthArgs) == 0 {
			t.Errorf("builtin %q has no health probe", s.Name)
			continue
		}
		if s.HealthArgs[0] != "--version" {
			t.Errorf("builtin %q probe = %v, want a --version probe", s.Name, s.HealthArgs)
		}
	}
}

func TestWriteVariant(t *testing.T) {
	// `echo base W p` — WithWrite appends WriteArgs before the prompt.
	base := New(Spec{Name: "fake", Bin: "echo", Args: []string{"base"}, WriteArgs: []string{"W"}})
	if !base.CanWrite() {
		t.Fatal("CanWrite should be true when WriteArgs are set")
	}
	got, err := base.WithWrite().Ask(context.Background(), "p")
	if err != nil {
		t.Fatal(err)
	}
	if got != "base W p" {
		t.Fatalf("write variant args = %q, want %q", got, "base W p")
	}
	// The base provider stays read-only (no write flag).
	if ro, _ := base.Ask(context.Background(), "p"); ro != "base p" {
		t.Fatalf("base provider = %q, want %q", ro, "base p")
	}

	plain := New(Spec{Name: "f", Bin: "echo"})
	if plain.CanWrite() {
		t.Fatal("CanWrite should be false without WriteArgs")
	}
}

func TestBuiltinsWriteArgs(t *testing.T) {
	want := map[string]bool{"claude": true, "cursor": true, "gemini": true, "codex": true, "agy": false}
	for _, s := range Builtins() {
		if got := len(s.WriteArgs) > 0; got != want[s.Name] {
			t.Errorf("builtin %q WriteArgs present = %v, want %v", s.Name, got, want[s.Name])
		}
	}
}

func TestErrorIncludesStderr(t *testing.T) {
	// `false` exits non-zero with no output.
	p := New(Spec{Name: "fake", Bin: "false"})
	if _, err := p.Ask(context.Background(), "x"); err == nil {
		t.Fatal("expected error from failing command")
	} else if !strings.Contains(err.Error(), "false") {
		t.Fatalf("error should name the binary: %v", err)
	}
}
