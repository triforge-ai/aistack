package store

import (
	"context"
	"testing"

	"ai-cli/internal/memory"
)

// TestPgVectorHybrid exercises BM25 keyword search and the Hybrid interface
// against a live database. Skipped unless AI_DATABASE_URL is set.
func TestPgVectorHybrid(t *testing.T) {
	ctx := context.Background()
	s := freshPgStore(t, 3)

	rows := []memory.Memory{
		{ID: "h1", WorkspaceID: "hybrid-ws", Type: memory.TypeDoc, Content: "ADR-001 documents the pgvector schema decision", Embedding: []float32{0, 0, 1}},
		{ID: "h2", WorkspaceID: "hybrid-ws", Type: memory.TypeDoc, Content: "semantic vector retrieval system design notes", Embedding: []float32{1, 0, 0}},
	}
	for _, m := range rows {
		if err := s.Save(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	// Verify the store satisfies the Hybrid interface.
	var _ Hybrid = s

	// Keyword search must find the exact identifier "ADR-001" even though its
	// embedding is orthogonal to a semantic query.
	kw, err := s.KeywordSearch(ctx, Query{WorkspaceID: "hybrid-ws", Text: "ADR-001", Limit: 10})
	if err != nil {
		t.Fatalf("keyword search: %v", err)
	}
	if len(kw) == 0 || kw[0].Memory.ID != "h1" {
		t.Fatalf("keyword search missed exact identifier: %+v", kw)
	}

	// Vector search ranks by embedding similarity.
	vec, err := s.VectorSearch(ctx, Query{WorkspaceID: "hybrid-ws", Embedding: []float32{0.9, 0, 0.1}, Limit: 10})
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(vec) == 0 || vec[0].Memory.ID != "h2" {
		t.Fatalf("vector search wrong top hit: %+v", vec)
	}
}
