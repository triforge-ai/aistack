package main

import (
	"testing"

	"github.com/triforge-ai/aistack/internal/memory"
	"github.com/triforge-ai/aistack/internal/workspace"
)

func TestDispatchUnknownCommand(t *testing.T) {
	if err := dispatch("bogus", nil); err == nil {
		t.Fatal("expected an error for an unknown command")
	}
}

func TestDispatchVersionAndHelp(t *testing.T) {
	// These take no workspace and must succeed without touching the filesystem.
	for _, cmd := range []string{"version", "--version", "-v", "help", "--help", "-h"} {
		if err := dispatch(cmd, nil); err != nil {
			t.Fatalf("dispatch(%q) = %v, want nil", cmd, err)
		}
	}
}

func TestParseOpts(t *testing.T) {
	opts, pos, err := parseOpts([]string{"backend", "--provider", "claude", "do", "--limit", "7", "it"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.provider != "claude" {
		t.Errorf("provider = %q, want claude", opts.provider)
	}
	if opts.limit != 7 {
		t.Errorf("limit = %d, want 7", opts.limit)
	}
	want := []string{"backend", "do", "it"}
	if len(pos) != len(want) {
		t.Fatalf("positional = %v, want %v", pos, want)
	}
	for i := range want {
		if pos[i] != want[i] {
			t.Errorf("positional[%d] = %q, want %q", i, pos[i], want[i])
		}
	}
}

func TestParseOptsErrors(t *testing.T) {
	cases := [][]string{
		{"--provider"},      // missing value
		{"--limit"},         // missing value
		{"--limit", "huge"}, // non-numeric
	}
	for _, args := range cases {
		if _, _, err := parseOpts(args); err == nil {
			t.Errorf("parseOpts(%v) = nil error, want error", args)
		}
	}
}

func TestFirstLine(t *testing.T) {
	cases := map[string]string{
		"  hello\nworld": "hello",
		"single":         "single",
		"\n\ntrim\nrest": "trim",
	}
	for in, want := range cases {
		if got := firstLine(in); got != want {
			t.Errorf("firstLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSyncSources(t *testing.T) {
	ws := &workspace.Workspace{Root: "/tmp/.ai"}
	if got := syncSources(ws); len(got) != 1 || got[0].Name != "documents" {
		t.Fatalf("without obsidian, want [documents], got %v", got)
	}

	ws.Obsidian = "/vault"
	got := syncSources(ws)
	if len(got) != 2 || got[1].Name != "obsidian" || got[1].Dir != "/vault" {
		t.Fatalf("with obsidian, want documents+obsidian, got %v", got)
	}
}

func TestFilterSources(t *testing.T) {
	ws := &workspace.Workspace{Root: "/tmp/.ai", Obsidian: "/vault"}
	srcs := syncSources(ws)

	if got := filterSources(srcs, "obsidian"); len(got) != 1 || got[0].Name != "obsidian" {
		t.Fatalf("filter obsidian = %v", got)
	}
	if got := filterSources(srcs, "nope"); len(got) != 0 {
		t.Fatalf("filter unknown should be empty, got %v", got)
	}
}

func TestAddNote(t *testing.T) {
	in := addNote("ws1", "remember me")
	if in.WorkspaceID != "ws1" || in.Content != "remember me" {
		t.Fatalf("addNote fields wrong: %+v", in)
	}
	if in.Type != memory.TypeNote || in.Source != memory.SourceCLI {
		t.Fatalf("addNote type/source wrong: %+v", in)
	}
	if name, _ := in.Metadata["name"].(string); name != "note" {
		t.Fatalf("addNote metadata name = %q, want note", name)
	}
}

func TestRunRequest(t *testing.T) {
	ws := &workspace.Workspace{ID: "ws"}
	req := runRequest(ws, "backend", "ship it", runOpts{provider: "claude", limit: 3})
	if req.Workspace != ws || req.Agent != "backend" || req.Task != "ship it" {
		t.Fatalf("runRequest core fields wrong: %+v", req)
	}
	if req.ProviderOverride != "claude" || req.MemoryLimit != 3 {
		t.Fatalf("runRequest opts mapping wrong: %+v", req)
	}
}
