package session

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Format is a session export encoding.
type Format string

const (
	FormatMarkdown Format = "md"
	FormatJSON     Format = "json"
)

// ParseFormat validates a user-supplied format string.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatMarkdown, "markdown":
		return FormatMarkdown, nil
	case FormatJSON:
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("unknown format %q (want md|json)", s)
	}
}

// Export renders a session record in the given format.
func Export(r Record, f Format) ([]byte, error) {
	switch f {
	case FormatJSON:
		return json.MarshalIndent(r, "", "  ")
	case FormatMarkdown:
		return []byte(renderMarkdown(r)), nil
	default:
		return nil, fmt.Errorf("unknown format %q", f)
	}
}

// renderMarkdown turns a session into a readable transcript.
func renderMarkdown(r Record) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", r.Name)
	fmt.Fprintf(&b, "- id: `%s`\n", r.ID)
	fmt.Fprintf(&b, "- workspace: %s\n", r.Workspace)
	fmt.Fprintf(&b, "- agent: %s\n", r.Agent)
	fmt.Fprintf(&b, "- provider: %s\n", r.Provider)
	fmt.Fprintf(&b, "- created: %s\n", time.Unix(r.CreatedAt, 0).Format(time.RFC3339))
	fmt.Fprintf(&b, "- updated: %s\n", time.Unix(r.UpdatedAt, 0).Format(time.RFC3339))
	fmt.Fprintf(&b, "- messages: %d\n\n", len(r.Messages))

	for _, m := range r.Messages {
		who := m.Role
		if m.Role == "assistant" && m.Provider != "" {
			who = "assistant (" + m.Provider + ")"
		}
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", who, strings.TrimRight(m.Text, "\n"))
	}
	return b.String()
}
