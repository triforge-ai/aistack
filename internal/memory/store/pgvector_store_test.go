package store

import (
	"context"
	"testing"

	"github.com/triforge-ai/aistack/internal/memory"
)

func TestVectorLiteral(t *testing.T) {
	cases := []struct {
		in   []float32
		want string
	}{
		{nil, "[]"},
		{[]float32{1, 0, 0.5}, "[1,0,0.5]"},
	}
	for _, c := range cases {
		if got := vectorLiteral(c.in); got != c.want {
			t.Errorf("vectorLiteral(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestPgVectorIntegration runs the real backend. It is skipped unless
// AI_DATABASE_URL points at a pgvector-enabled Postgres, e.g.:
//
//	AI_DATABASE_URL="host=localhost port=5432 user=ai password=ai dbname=ai_workspace sslmode=disable" go test ./...
func TestPgVectorIntegration(t *testing.T) {
	ctx := context.Background()
	s := freshPgStore(t, 3)

	docs := map[string][]float32{
		"vectors and pgvector retrieval": {1, 0, 0},
		"baking bread at home":           {0, 1, 0},
	}
	for content, emb := range docs {
		if err := s.Save(ctx, memory.Memory{
			ID:          content,
			WorkspaceID: "test-ws",
			Type:        memory.TypeNote,
			Content:     content,
			Embedding:   emb,
			Metadata:    map[string]any{"k": "v"},
		}); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	hits, err := s.Search(ctx, Query{WorkspaceID: "test-ws", Embedding: []float32{0.9, 0.1, 0}, Limit: 1})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "vectors and pgvector retrieval" {
		t.Fatalf("nearest neighbour wrong: %+v", hits)
	}
	if hits[0].Metadata["k"] != "v" {
		t.Fatalf("metadata round-trip failed: %+v", hits[0].Metadata)
	}

	// Workspace scoping: other workspace must not appear.
	other, _ := s.Search(ctx, Query{WorkspaceID: "nobody", Embedding: []float32{1, 0, 0}, Limit: 5})
	if len(other) != 0 {
		t.Fatalf("workspace scoping leaked: %+v", other)
	}
}
