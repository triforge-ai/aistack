// Package render is the presentation consumer of the unified provider.Event
// stream. Backends emit typed events (text, thinking, tool-use, tool-result,
// status, error, log) and never touch the terminal themselves; a renderer here
// decides how — or whether — to display each event. Keeping rendering out of the
// provider lets the same event stream feed a terminal, a transcript recorder, or
// a structured log without the backend knowing the difference.
package render

import (
	"fmt"
	"io"

	"github.com/triforge-ai/aistack/internal/provider"
)

// Terminal returns an Event sink that renders the stream to w for a human reader:
// assistant text inline, tool calls on their own dimmed line, and errors in red.
// Thinking, status, tool-result, and log events are intentionally not shown here
// — they remain available to other consumers of the same stream — so the
// terminal stays focused on the agent's answer and actions.
func Terminal(w io.Writer) func(provider.Event) {
	return func(e provider.Event) {
		switch e.Kind {
		case provider.EventText:
			fmt.Fprint(w, e.Text)
		case provider.EventToolUse:
			fmt.Fprintf(w, "\n\x1b[2m· %s %s\x1b[0m\n", e.Tool, summarize(e.Input))
		case provider.EventError:
			fmt.Fprintf(w, "\n\x1b[31m%s\x1b[0m\n", e.Text)
		}
	}
}

// summarize extracts a short, human-readable hint from a tool's decoded input
// (e.g. the file path or command it is acting on) for the dimmed tool-call line.
func summarize(m map[string]any) string {
	if m == nil {
		return ""
	}
	for _, k := range []string{"file_path", "path", "command", "pattern", "url", "query"} {
		if v, ok := m[k].(string); ok && v != "" {
			if len(v) > 80 {
				v = v[:77] + "…"
			}
			return v
		}
	}
	return ""
}
