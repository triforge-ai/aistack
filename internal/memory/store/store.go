// Package store abstracts the persistence layer for memories so the DB is
// fully replaceable. The default implementation keeps everything in memory;
// PgVectorStore swaps in for production semantic retrieval.
package store

import (
	"context"

	"github.com/triforge-ai/aistack/internal/memory"
)

// Query narrows a search to a workspace and limits the result set.
type Query struct {
	WorkspaceID string
	Embedding   []float32
	// Text is the raw query string, used by keyword (BM25) search. Vector-only
	// backends ignore it.
	Text  string
	Limit int
}

// Scored pairs a memory with a retriever's relevance score.
type Scored struct {
	Memory memory.Memory
	Score  float64
}

// Store persists memories and answers nearest-neighbour searches over their
// embeddings.
type Store interface {
	Save(ctx context.Context, m memory.Memory) error
	Search(ctx context.Context, q Query) ([]memory.Memory, error)
	Delete(ctx context.Context, id string) error
}

// Lister is an optional capability for backends that can enumerate memories.
// It is kept separate from Store so the core persistence contract stays minimal
// (a plain SELECT, no retrieval intelligence).
type Lister interface {
	List(ctx context.Context, workspaceID string) ([]memory.Memory, error)
}

// Hybrid is an optional capability for backends that support both semantic
// (vector) and keyword (BM25/full-text) retrieval. Each method returns scored
// candidates; fusion happens in the ranking layer above the store, so the store
// stays free of ranking policy. Both queries always filter by workspace_id.
type Hybrid interface {
	VectorSearch(ctx context.Context, q Query) ([]Scored, error)
	KeywordSearch(ctx context.Context, q Query) ([]Scored, error)
}
