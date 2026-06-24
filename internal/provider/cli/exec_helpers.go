package cli

import (
	"io"
	"os"
	"strings"
)

// stderrTailBytes bounds how much trailing stderr we keep to attach to an error
// message. A few KB is enough to capture a CLI panic or stack tail without
// holding an unbounded buffer for a chatty process.
const stderrTailBytes = 8 * 1024

// stderrTail is an io.Writer that forwards everything to an optional inner
// writer (e.g. os.Stderr for live diagnostics) while retaining the last
// stderrTailBytes for error reporting. When a CLI exits non-zero, an
// exit-code-only message is useless; the retained tail carries the real reason.
type stderrTail struct {
	inner io.Writer
	buf   []byte
}

func newStderrTail(inner io.Writer) *stderrTail {
	return &stderrTail{inner: inner}
}

func (w *stderrTail) Write(p []byte) (int, error) {
	if w.inner != nil {
		_, _ = w.inner.Write(p)
	}
	w.buf = append(w.buf, p...)
	if len(w.buf) > stderrTailBytes {
		w.buf = w.buf[len(w.buf)-stderrTailBytes:]
	}
	return len(p), nil
}

// Tail returns the retained trailing stderr, trimmed.
func (w *stderrTail) Tail() string {
	return strings.TrimSpace(string(w.buf))
}

// withStderr joins a base message with a stderr tail, omitting either when empty.
func withStderr(msg, tail string) string {
	msg = strings.TrimSpace(msg)
	tail = strings.TrimSpace(tail)
	switch {
	case tail == "":
		return msg
	case msg == "":
		return tail
	default:
		return msg + "\n" + tail
	}
}

// filteredEnv returns the parent environment with internal Claude Code session
// markers removed, so a spawned agent never mistakes itself for a nested or
// resumed session (or inherits the parent's transport/exec path). The
// user-facing CLAUDE_CODE_* configuration namespace is deliberately preserved —
// blanket-stripping it is what historically broke CLIs that rely on documented
// overrides such as CLAUDE_CODE_GIT_BASH_PATH.
func filteredEnv() []string {
	base := os.Environ()
	out := make([]string, 0, len(base))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if isInternalSessionEnv(key) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func isInternalSessionEnv(key string) bool {
	switch key {
	case "CLAUDECODE", // "1" when running inside Claude Code
		"CLAUDE_CODE_ENTRYPOINT", // entrypoint marker
		"CLAUDE_CODE_EXECPATH",   // path to the running CLI binary
		"CLAUDE_CODE_SESSION_ID", // per-session identifier
		"CLAUDE_CODE_SSE_PORT":   // IDE-extension transport port
		return true
	}
	// CLAUDECODE_* (no underscore between CLAUDE and CODE) is wholly internal.
	return strings.HasPrefix(key, "CLAUDECODE_")
}
