package cli

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/triforge-ai/aistack/internal/provider"
)

// This file parses Claude Code's newline-delimited "stream-json" event protocol
// into backend-agnostic provider.Events, while accumulating the assistant text,
// per-model token usage, the backend session id, and the final result. The
// parser is intentionally tolerant: malformed or unknown lines are skipped, the
// same way the CLI's own renderer copes with interleaved non-JSON noise.

// sjMessage is the subset of a stream-json envelope we consume.
type sjMessage struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	SessionID string          `json:"session_id"`
	Model     string          `json:"model"`
	Message   json.RawMessage `json:"message"`

	// result-event fields
	Result     string                  `json:"result"`
	IsError    bool                    `json:"is_error"`
	Usage      *sjUsage                `json:"usage"`
	ModelUsage map[string]sjModelUsage `json:"modelUsage"`
}

// sjContent is the inner assistant/user message payload.
type sjContent struct {
	Role    string    `json:"role"`
	Model   string    `json:"model"`
	Content []sjBlock `json:"content"`
	Usage   *sjUsage  `json:"usage"`
}

type sjBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
}

// sjUsage matches the snake_case usage block on streamed assistant events.
type sjUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

// sjModelUsage matches the camelCase per-model usage map on the final result.
type sjModelUsage struct {
	InputTokens              int64 `json:"inputTokens"`
	OutputTokens             int64 `json:"outputTokens"`
	CacheReadInputTokens     int64 `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int64 `json:"cacheCreationInputTokens"`
}

// sjResult is the accumulated outcome of parsing one stream.
type sjResult struct {
	output    strings.Builder
	usage     map[string]provider.Usage
	sessionID string
	finalText string
	isError   bool
}

// text returns the best available assistant text: the explicit final result
// when present, otherwise the accumulated assistant chunks.
func (r *sjResult) text() string {
	if r.finalText != "" {
		return r.finalText
	}
	return strings.TrimSpace(r.output.String())
}

// parseStreamJSON consumes the event stream from r, emitting a typed
// provider.Event for each occurrence via onEvent (nil-safe) and accumulating
// text, per-model usage, session id, and the final result.
func parseStreamJSON(r io.Reader, onEvent func(provider.Event)) sjResult {
	res := sjResult{usage: map[string]provider.Usage{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var msg sjMessage
		if json.Unmarshal([]byte(line), &msg) != nil {
			continue // ignore non-JSON / unknown lines
		}

		switch msg.Type {
		case "system":
			if msg.SessionID != "" {
				res.sessionID = msg.SessionID
			}
			emit(onEvent, provider.Event{Kind: provider.EventStatus, Status: "running", SessionID: res.sessionID})
		case "assistant":
			handleAssistant(msg, onEvent, &res)
		case "user":
			handleUser(msg, onEvent)
		case "result":
			if msg.SessionID != "" {
				res.sessionID = msg.SessionID
			}
			res.finalText = msg.Result
			res.isError = msg.IsError
			if u := resultUsage(msg); len(u) > 0 {
				res.usage = u
			}
		}
	}
	return res
}

// handleAssistant accumulates per-model usage and emits text/thinking/tool-use
// events for each content block of an assistant message.
func handleAssistant(msg sjMessage, onEvent func(provider.Event), res *sjResult) {
	var c sjContent
	if json.Unmarshal(msg.Message, &c) != nil {
		return
	}
	if c.Usage != nil && c.Model != "" {
		mergeUsage(res.usage, c.Model, c.Usage)
	}
	for _, b := range c.Content {
		switch b.Type {
		case "text":
			if b.Text != "" {
				res.output.WriteString(b.Text)
				emit(onEvent, provider.Event{Kind: provider.EventText, Text: b.Text})
			}
		case "thinking":
			if b.Text != "" {
				emit(onEvent, provider.Event{Kind: provider.EventThinking, Text: b.Text})
			}
		case "tool_use":
			emit(onEvent, provider.Event{
				Kind:   provider.EventToolUse,
				Tool:   b.Name,
				CallID: b.ID,
				Input:  decodeInput(b.Input),
			})
		}
	}
}

// handleUser emits tool-result events carried back on user messages.
func handleUser(msg sjMessage, onEvent func(provider.Event)) {
	var c sjContent
	if json.Unmarshal(msg.Message, &c) != nil {
		return
	}
	for _, b := range c.Content {
		if b.Type == "tool_result" {
			emit(onEvent, provider.Event{
				Kind:   provider.EventToolResult,
				CallID: b.ToolUseID,
				Output: string(b.Content),
			})
		}
	}
}

// resultUsage prefers the per-model breakdown on the result event, falling back
// to the single aggregate usage keyed by the event's model.
func resultUsage(msg sjMessage) map[string]provider.Usage {
	if len(msg.ModelUsage) > 0 {
		out := make(map[string]provider.Usage, len(msg.ModelUsage))
		for model, u := range msg.ModelUsage {
			usage := provider.Usage{
				InputTokens:      u.InputTokens,
				OutputTokens:     u.OutputTokens,
				CacheReadTokens:  u.CacheReadInputTokens,
				CacheWriteTokens: u.CacheCreationInputTokens,
			}
			if model != "" && hasTokens(usage) {
				out[model] = usage
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if msg.Usage == nil || msg.Model == "" {
		return nil
	}
	out := map[string]provider.Usage{}
	mergeUsage(out, msg.Model, msg.Usage)
	return out
}

// mergeUsage adds u's tokens into the running total for model.
func mergeUsage(into map[string]provider.Usage, model string, u *sjUsage) {
	cur := into[model]
	cur.InputTokens += u.InputTokens
	cur.OutputTokens += u.OutputTokens
	cur.CacheReadTokens += u.CacheReadInputTokens
	cur.CacheWriteTokens += u.CacheCreationInputTokens
	into[model] = cur
}

func hasTokens(u provider.Usage) bool {
	return u.InputTokens > 0 || u.OutputTokens > 0 || u.CacheReadTokens > 0 || u.CacheWriteTokens > 0
}

func decodeInput(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
}

func emit(onEvent func(provider.Event), e provider.Event) {
	if onEvent != nil {
		onEvent(e)
	}
}
