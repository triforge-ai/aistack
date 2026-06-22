package sync_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ai-cli/internal/memory/embed"
	"ai-cli/internal/memory/service"
	"ai-cli/internal/memory/store"
	"ai-cli/internal/memory/sync"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSyncIncremental(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	docs := filepath.Join(dir, "documents")
	state := filepath.Join(dir, ".cache")

	st := store.NewMemoryStore()
	svc := service.New(st, embed.NewHashEmbedder(64))
	syncer := sync.New(svc, state)
	src := sync.Source{Name: "documents", Dir: docs}

	write(t, filepath.Join(docs, "a.md"), "alpha")
	write(t, filepath.Join(docs, "b.md"), "beta")

	// First sync: both files added.
	rep, err := syncer.Sync(ctx, "ws", src)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != 2 || rep.Updated != 0 || rep.Removed != 0 {
		t.Fatalf("first sync: %+v", rep)
	}

	// No changes: everything unchanged.
	rep, _ = syncer.Sync(ctx, "ws", src)
	if rep.Unchanged != 2 || rep.Added != 0 {
		t.Fatalf("noop sync: %+v", rep)
	}

	// Modify a, add c, remove b.
	write(t, filepath.Join(docs, "a.md"), "alpha edited")
	write(t, filepath.Join(docs, "c.md"), "gamma")
	if err := os.Remove(filepath.Join(docs, "b.md")); err != nil {
		t.Fatal(err)
	}
	rep, _ = syncer.Sync(ctx, "ws", src)
	if rep.Added != 1 || rep.Updated != 1 || rep.Removed != 1 {
		t.Fatalf("change sync: %+v", rep)
	}

	// Store should now hold a (edited), c — i.e. 2 docs.
	all, err := st.List(ctx, "ws")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 memories after reconcile, got %d", len(all))
	}
}

func TestSyncMissingDirIsEmpty(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	svc := service.New(store.NewMemoryStore(), embed.NewHashEmbedder(64))
	syncer := sync.New(svc, filepath.Join(dir, ".cache"))

	rep, err := syncer.Sync(ctx, "ws", sync.Source{Name: "obsidian", Dir: filepath.Join(dir, "nope")})
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if rep.Added != 0 {
		t.Fatalf("missing dir should add nothing: %+v", rep)
	}
}
