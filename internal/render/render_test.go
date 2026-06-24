package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/triforge-ai/aistack/internal/provider"
)

// TestTerminalRendersTextAndToolUse checks the human renderer shows assistant
// text inline and a tool call (name + summarized input) on its own line.
func TestTerminalRendersTextAndToolUse(t *testing.T) {
	var out bytes.Buffer
	sink := Terminal(&out)
	sink(provider.Event{Kind: provider.EventStatus, Status: "running"})
	sink(provider.Event{Kind: provider.EventToolUse, Tool: "Write", Input: map[string]any{
		"file_path": "landing/index.html", "content": "<html>",
	}})
	sink(provider.Event{Kind: provider.EventText, Text: "Done — wrote the page."})

	rendered := out.String()
	if !strings.Contains(rendered, "Write") || !strings.Contains(rendered, "landing/index.html") {
		t.Fatalf("tool call not rendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Done — wrote the page.") {
		t.Fatalf("assistant text not rendered:\n%s", rendered)
	}
}

// TestTerminalRendersError checks error events are surfaced to the reader.
func TestTerminalRendersError(t *testing.T) {
	var out bytes.Buffer
	Terminal(&out)(provider.Event{Kind: provider.EventError, Text: "boom"})
	if !strings.Contains(out.String(), "boom") {
		t.Fatalf("error not rendered: %q", out.String())
	}
}

// TestTerminalSkipsNonDisplayEvents checks the terminal renderer stays focused
// on the answer: thinking, tool-result, status, and log events emit nothing.
func TestTerminalSkipsNonDisplayEvents(t *testing.T) {
	var out bytes.Buffer
	sink := Terminal(&out)
	sink(provider.Event{Kind: provider.EventThinking, Text: "secret"})
	sink(provider.Event{Kind: provider.EventToolResult, Output: "secret"})
	sink(provider.Event{Kind: provider.EventStatus, Status: "running"})
	sink(provider.Event{Kind: provider.EventLog, Text: "secret"})
	if out.Len() != 0 {
		t.Fatalf("expected no output, got %q", out.String())
	}
}

// TestSummarize checks the dimmed tool-call hint picks the most useful field
// from decoded input, and is empty when none applies.
func TestSummarize(t *testing.T) {
	cases := []struct {
		in   map[string]any
		want string
	}{
		{map[string]any{"file_path": "a/b.go"}, "a/b.go"},
		{map[string]any{"command": "go test ./..."}, "go test ./..."},
		{map[string]any{"other": "x"}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := summarize(c.in); got != c.want {
			t.Errorf("summarize(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
