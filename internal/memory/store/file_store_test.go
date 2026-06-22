package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/triforge-ai/aistack/internal/memory"
	"github.com/triforge-ai/aistack/internal/memory/store"
)

func TestFileStorePersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "memory.json")

	s1, err := store.NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	m := memory.Memory{ID: "1", WorkspaceID: "ws", Type: memory.TypeNote, Content: "hello", Embedding: []float32{1, 0}}
	if err := s1.Save(ctx, m); err != nil {
		t.Fatal(err)
	}

	// Reopen from disk — data must survive.
	s2, err := store.NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.List(ctx, "ws")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Content != "hello" {
		t.Fatalf("reopen lost data: %+v", got)
	}

	if err := s2.Delete(ctx, "1"); err != nil {
		t.Fatal(err)
	}
	s3, _ := store.NewFileStore(path)
	got, _ = s3.List(ctx, "ws")
	if len(got) != 0 {
		t.Fatalf("delete not persisted: %+v", got)
	}
}
