package session_test

import (
	"context"
	"errors"
	"testing"

	"github.com/triforge-ai/aistack/internal/session"
)

func TestNewDefaultsName(t *testing.T) {
	r := session.New("", "ws", "backend", "claude")
	if r.ID == "" || r.CreatedAt == 0 || r.UpdatedAt == 0 {
		t.Fatalf("New left identity/timestamps unset: %+v", r)
	}
	if r.Name == "" {
		t.Fatal("New should derive a name when none is given")
	}

	named := session.New("my-chat", "ws", "backend", "claude")
	if named.Name != "my-chat" {
		t.Fatalf("explicit name not kept: %q", named.Name)
	}
}

func TestAppendAdvancesUpdatedAt(t *testing.T) {
	r := session.New("s", "ws", "backend", "claude")
	r.UpdatedAt = 0 // force a known baseline
	r.Append(session.Message{Role: "user", Text: "hi", Ts: 100})
	r.Append(session.Message{Role: "assistant", Provider: "claude", Text: "hello", Ts: 200})
	if len(r.Messages) != 2 {
		t.Fatalf("want 2 messages, got %d", len(r.Messages))
	}
	if r.UpdatedAt != 200 {
		t.Fatalf("UpdatedAt = %d, want 200 (latest message)", r.UpdatedAt)
	}
}

func TestFileStorePersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	s1 := session.NewFileStore(dir)
	rec := session.New("first", "ws", "backend", "claude")
	rec.Append(session.Message{Role: "user", Text: "remember this"})
	rec.Append(session.Message{Role: "assistant", Provider: "claude", Text: "noted"})
	if err := s1.Save(ctx, rec); err != nil {
		t.Fatal(err)
	}

	// Reopen from disk — the transcript must survive.
	s2 := session.NewFileStore(dir)
	got, err := s2.Load(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "first" || len(got.Messages) != 2 || got.Messages[1].Text != "noted" {
		t.Fatalf("reopen lost data: %+v", got)
	}
}

func TestListSortsByRecencyAndFiltersWorkspace(t *testing.T) {
	ctx := context.Background()
	s := session.NewFileStore(t.TempDir())

	older := session.New("older", "ws", "backend", "claude")
	older.UpdatedAt = 100
	newer := session.New("newer", "ws", "backend", "claude")
	newer.UpdatedAt = 200
	other := session.New("other", "ws2", "backend", "claude")
	other.UpdatedAt = 300

	for _, r := range []session.Record{older, newer, other} {
		if err := s.Save(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	list, err := s.List(ctx, "ws")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("workspace filter failed: got %d, want 2", len(list))
	}
	if list[0].Name != "newer" || list[1].Name != "older" {
		t.Fatalf("not sorted most-recent-first: %q, %q", list[0].Name, list[1].Name)
	}
}

func TestLoadNotFound(t *testing.T) {
	s := session.NewFileStore(t.TempDir())
	if _, err := s.Load(context.Background(), session.NewID()); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	s := session.NewFileStore(t.TempDir())
	rec := session.New("gone", "ws", "backend", "claude")
	if err := s.Save(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, rec.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(ctx, rec.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("session should be gone, got %v", err)
	}
	if err := s.Delete(ctx, rec.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("deleting twice should report not found, got %v", err)
	}
}

func TestInvalidIDRejected(t *testing.T) {
	ctx := context.Background()
	s := session.NewFileStore(t.TempDir())
	for _, bad := range []string{"", "../escape", "a/b", `x\y`} {
		if _, err := s.Load(ctx, bad); err == nil {
			t.Errorf("Load(%q) should reject an unsafe id", bad)
		}
	}
}
