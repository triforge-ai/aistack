package service_test

import (
	"context"
	"testing"

	"github.com/triforge-ai/aistack/internal/memory"
	"github.com/triforge-ai/aistack/internal/memory/embed"
	"github.com/triforge-ai/aistack/internal/memory/service"
	"github.com/triforge-ai/aistack/internal/memory/store"
)

func newService() *service.Service {
	return service.New(store.NewMemoryStore(), embed.NewHashEmbedder(256))
}

func TestAddAndSearchRanksRelevant(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	add := func(content string) {
		if _, err := svc.Add(ctx, service.AddInput{
			WorkspaceID: "ws",
			Type:        memory.TypeNote,
			Source:      memory.SourceCLI,
			Content:     content,
		}); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	add("postgres pgvector cosine similarity retrieval")
	add("baking sourdough bread at home with a dutch oven")

	hits, err := svc.Search(ctx, "ws", "vector database semantic retrieval", 1)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d", len(hits))
	}
	if want := "postgres"; hits[0].Content[:len(want)] != want {
		t.Fatalf("most relevant hit should be the pgvector note, got %q", hits[0].Content)
	}
}

func TestSearchIsWorkspaceScoped(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	if _, err := svc.Add(ctx, service.AddInput{WorkspaceID: "a", Type: memory.TypeNote, Content: "alpha content"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(ctx, service.AddInput{WorkspaceID: "b", Type: memory.TypeNote, Content: "beta content"}); err != nil {
		t.Fatal(err)
	}

	hits, err := svc.Search(ctx, "a", "content", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.WorkspaceID != "a" {
			t.Fatalf("search leaked workspace %q", h.WorkspaceID)
		}
	}
}
