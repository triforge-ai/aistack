package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/triforge-ai/aistack/internal/provider"
)

func TestExecuteStreamJSONStructured(t *testing.T) {
	// A stand-in agent CLI (sh script) emits a representative stream-json stream;
	// Stream is false so Execute does not render to the terminal during the test.
	p := New(Spec{Name: "fake", Bin: "sh", Args: []string{"testdata/fake_streamjson.sh"}, Format: "stream-json"})

	var kinds []provider.EventKind
	res, err := p.Execute(context.Background(), "ignored prompt", provider.ExecOptions{
		OnEvent: func(e provider.Event) { kinds = append(kinds, e.Kind) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "completed" {
		t.Fatalf("status = %q, want completed", res.Status)
	}
	if res.Output != "Done." {
		t.Fatalf("output = %q, want Done.", res.Output)
	}
	if res.SessionID != "sess-xyz" {
		t.Fatalf("sessionID = %q, want sess-xyz", res.SessionID)
	}
	if u := res.Usage["m"]; u.InputTokens != 120 || u.OutputTokens != 30 {
		t.Fatalf("usage[m] = %+v, want input=120 output=30", u)
	}
	// Status, tool-use, then text were surfaced as typed events.
	if len(kinds) < 3 || kinds[0] != provider.EventStatus {
		t.Fatalf("event kinds = %v", kinds)
	}
}

func TestParseStreamJSONStructured(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"sess-123"}`,
		`{"type":"assistant","message":{"model":"claude-opus","content":[{"type":"thinking","text":"hmm"},{"type":"tool_use","id":"t1","name":"Write","input":{"file_path":"a.go"}}],"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":2}}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`,
		`{"type":"assistant","message":{"model":"claude-opus","content":[{"type":"text","text":"Done."}]}}`,
		`{"type":"result","subtype":"success","session_id":"sess-123","result":"Done.","modelUsage":{"claude-opus":{"inputTokens":100,"outputTokens":40,"cacheReadInputTokens":8}}}`,
	}, "\n")

	var events []provider.Event
	res := parseStreamJSON(strings.NewReader(stream), func(e provider.Event) {
		events = append(events, e)
	})

	if res.sessionID != "sess-123" {
		t.Fatalf("sessionID = %q, want sess-123", res.sessionID)
	}
	if res.text() != "Done." {
		t.Fatalf("text = %q, want Done.", res.text())
	}
	// The result event's per-model usage supersedes the streamed aggregate.
	u, ok := res.usage["claude-opus"]
	if !ok {
		t.Fatalf("no usage for claude-opus: %+v", res.usage)
	}
	if u.InputTokens != 100 || u.OutputTokens != 40 || u.CacheReadTokens != 8 {
		t.Fatalf("usage = %+v, want input=100 output=40 cacheRead=8", u)
	}

	// Typed events were emitted in order with the right kinds.
	var kinds []provider.EventKind
	for _, e := range events {
		kinds = append(kinds, e.Kind)
	}
	wantKinds := []provider.EventKind{
		provider.EventStatus, provider.EventThinking, provider.EventToolUse,
		provider.EventToolResult, provider.EventText,
	}
	if len(kinds) != len(wantKinds) {
		t.Fatalf("event kinds = %v, want %v", kinds, wantKinds)
	}
	for i := range wantKinds {
		if kinds[i] != wantKinds[i] {
			t.Fatalf("event[%d] = %q, want %q (all: %v)", i, kinds[i], wantKinds[i], kinds)
		}
	}
	// The tool-use event carried decoded input.
	if events[2].Tool != "Write" || events[2].Input["file_path"] != "a.go" {
		t.Fatalf("tool-use event = %+v", events[2])
	}
}

func TestParseStreamJSONUsageFallback(t *testing.T) {
	// No modelUsage map: fall back to the aggregate usage keyed by the event model.
	stream := `{"type":"result","model":"gpt","result":"x","usage":{"input_tokens":7,"output_tokens":3}}`
	res := parseStreamJSON(strings.NewReader(stream), nil)
	u, ok := res.usage["gpt"]
	if !ok || u.InputTokens != 7 || u.OutputTokens != 3 {
		t.Fatalf("fallback usage = %+v (ok=%v)", u, ok)
	}
}
