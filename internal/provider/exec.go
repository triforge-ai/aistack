package provider

import (
	"context"
	"time"
)

// EventKind classifies a streamed Event emitted by a backend during a run.
type EventKind string

const (
	EventText       EventKind = "text"        // assistant text chunk
	EventThinking   EventKind = "thinking"    // extended-thinking chunk
	EventToolUse    EventKind = "tool-use"    // the agent invoked a tool
	EventToolResult EventKind = "tool-result" // a tool returned
	EventStatus     EventKind = "status"      // lifecycle status (e.g. "running")
	EventError      EventKind = "error"       // backend-reported error text
	EventLog        EventKind = "log"         // backend diagnostic log line
)

// Event is a single, backend-agnostic occurrence during a run. Backends that
// speak a structured protocol (e.g. Claude Code's stream-json) translate their
// native events into this shape so callers render progress uniformly and can
// record a transcript without re-parsing vendor formats.
type Event struct {
	Kind      EventKind
	Text      string         // content for Text/Thinking/Error/Log
	Tool      string         // tool name for ToolUse/ToolResult
	CallID    string         // tool call id for ToolUse/ToolResult
	Input     map[string]any // tool input for ToolUse
	Output    string         // tool output for ToolResult
	Status    string         // status label for Status
	Level     string         // log level for Log
	SessionID string         // backend session id, once known (Status)
}

// Usage tracks token consumption for one model within a run.
type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
}

// RunResult is the final outcome of an Execute call. It carries everything Ask
// returned (the text) plus the structured signal a personal AI OS needs:
// per-model token usage for cost accounting, a resumable session id, and a
// status/error pair distinguishing a clean finish from a timeout or failure.
type RunResult struct {
	Output     string           // accumulated assistant text (or final result)
	Status     string           // "completed" | "failed" | "timeout" | "aborted"
	Error      string           // failure detail; empty on success
	SessionID  string           // backend session id, for later --resume
	DurationMs int64            // wall-clock duration of the run
	Usage      map[string]Usage // token usage keyed by model name
}

// ExecOptions configures a single Execute call. Zero values are safe: an empty
// Model uses the backend default, a zero Timeout falls back to the provider's
// own configured timeout, and a nil OnEvent simply disables live streaming
// (the RunResult is still returned).
type ExecOptions struct {
	Model           string        // model override; empty uses the backend default
	ResumeSessionID string        // resume a prior backend session when non-empty
	SystemPrompt    string        // appended system/developer instructions
	Write           bool          // grant file/tool write permission for this run
	Timeout         time.Duration // hard wall-clock bound; 0 uses the provider default
	OnEvent         func(Event)   // receives typed progress events as they arrive
}

// Executor is an optional capability: a provider that can run a prompt and
// return structured results (token usage, a resumable session id, a typed event
// stream) in addition to plain text. Backends speaking a structured protocol
// implement this; the simple Ask path remains for plain-text CLIs and for
// callers that only need the final string.
type Executor interface {
	Execute(ctx context.Context, prompt string, opts ExecOptions) (RunResult, error)
}
