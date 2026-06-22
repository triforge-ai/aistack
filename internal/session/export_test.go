package session_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/triforge-ai/aistack/internal/session"
)

func sampleRecord() session.Record {
	r := session.New("design-chat", "ws", "backend", "claude")
	r.Append(session.Message{Role: "user", Text: "how do I shard?"})
	r.Append(session.Message{Role: "assistant", Provider: "claude", Text: "use a hash ring"})
	return r
}

func TestExportMarkdown(t *testing.T) {
	out, err := session.Export(sampleRecord(), session.FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	md := string(out)
	for _, want := range []string{"# design-chat", "## user", "how do I shard?", "## assistant (claude)", "use a hash ring"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown export missing %q:\n%s", want, md)
		}
	}
}

func TestExportJSONRoundTrips(t *testing.T) {
	rec := sampleRecord()
	out, err := session.Export(rec, session.FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var back session.Record
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("json export is not valid: %v", err)
	}
	if back.ID != rec.ID || len(back.Messages) != 2 || back.Messages[1].Text != "use a hash ring" {
		t.Fatalf("json export lost data: %+v", back)
	}
}

func TestParseFormat(t *testing.T) {
	for _, ok := range []string{"md", "markdown", "json"} {
		if _, err := session.ParseFormat(ok); err != nil {
			t.Errorf("ParseFormat(%q) errored: %v", ok, err)
		}
	}
	if _, err := session.ParseFormat("yaml"); err == nil {
		t.Error("ParseFormat should reject an unknown format")
	}
}
