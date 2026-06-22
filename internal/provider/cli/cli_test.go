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

func TestErrorIncludesStderr(t *testing.T) {
	// `false` exits non-zero with no output.
	p := New(Spec{Name: "fake", Bin: "false"})
	if _, err := p.Ask(context.Background(), "x"); err == nil {
		t.Fatal("expected error from failing command")
	} else if !strings.Contains(err.Error(), "false") {
		t.Fatalf("error should name the binary: %v", err)
	}
}
