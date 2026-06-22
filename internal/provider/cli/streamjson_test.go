package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderStreamJSON(t *testing.T) {
	// A representative Claude Code stream: init noise, a tool call, text, result.
	stream := strings.Join([]string{
		`{"type":"system","subtype":"init","tools":["Write"]}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"landing/index.html","content":"<html>"}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Done — wrote the page."}]}}`,
		`not-json-should-be-ignored`,
		`{"type":"result","subtype":"success","result":"Done — wrote the page."}`,
	}, "\n")

	var out bytes.Buffer
	got := renderStreamJSON(strings.NewReader(stream), &out)

	if got != "Done — wrote the page." {
		t.Fatalf("final result = %q", got)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "Write") || !strings.Contains(rendered, "landing/index.html") {
		t.Fatalf("tool call not rendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Done — wrote the page.") {
		t.Fatalf("assistant text not rendered:\n%s", rendered)
	}
}

func TestToolSummary(t *testing.T) {
	cases := map[string]string{
		`{"file_path":"a/b.go"}`:      "a/b.go",
		`{"command":"go test ./..."}`: "go test ./...",
		`{"other":"x"}`:               "",
	}
	for in, want := range cases {
		if got := toolSummary([]byte(in)); got != want {
			t.Errorf("toolSummary(%s) = %q, want %q", in, got, want)
		}
	}
}
