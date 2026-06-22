// Package service orchestrates the memory engine: it embeds content on the way
// in and embeds queries on the way out, keeping callers free of vector details.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"ai-cli/internal/memory"
	"ai-cli/internal/memory/chunk"
	"ai-cli/internal/memory/embed"
	"ai-cli/internal/memory/ranking"
	"ai-cli/internal/memory/store"
)

// Service is the high-level entrypoint to the memory engine.
type Service struct {
	store    store.Store
	embedder embed.Embedder
}

// New wires a Service from a store and an embedder.
func New(s store.Store, e embed.Embedder) *Service {
	return &Service{store: s, embedder: e}
}

// AddInput describes a piece of knowledge to remember.
type AddInput struct {
	WorkspaceID string
	Type        memory.Type
	Source      memory.Source
	Content     string
	Metadata    map[string]any
}

// Add chunks the content, embeds each chunk, and stores it. It returns the IDs
// of the stored memories (one per chunk).
func (s *Service) Add(ctx context.Context, in AddInput) ([]string, error) {
	chunks := chunk.Chunk(in.Content, chunk.DefaultOptions())
	if len(chunks) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(chunks))
	for i, c := range chunks {
		vec, err := s.embedder.Embed(c)
		if err != nil {
			return nil, fmt.Errorf("embed chunk %d: %w", i, err)
		}

		meta := map[string]any{}
		for k, v := range in.Metadata {
			meta[k] = v
		}
		if len(chunks) > 1 {
			meta["chunk"] = i
		}

		m := memory.Memory{
			ID:          newID(),
			WorkspaceID: in.WorkspaceID,
			Type:        in.Type,
			Source:      in.Source,
			Content:     c,
			Embedding:   vec,
			Metadata:    meta,
			CreatedAt:   time.Now().Unix(),
		}
		if err := s.store.Save(ctx, m); err != nil {
			return nil, fmt.Errorf("save chunk %d: %w", i, err)
		}
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// Search embeds the query and returns the most relevant memories. When the
// store supports hybrid retrieval (vector + keyword), results are fused;
// otherwise it falls back to pure vector search. The Context Builder consumes
// the returned memories and never touches the store directly.
func (s *Service) Search(ctx context.Context, workspaceID, query string, limit int) ([]memory.Memory, error) {
	vec, err := s.embedder.Embed(query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	q := store.Query{
		WorkspaceID: workspaceID,
		Embedding:   vec,
		Text:        query,
		Limit:       limit,
	}
	if h, ok := s.store.(store.Hybrid); ok {
		return s.hybridSearch(ctx, h, q, limit)
	}
	return s.store.Search(ctx, q)
}

// overscan is how many candidates each retriever fetches before fusion, so
// fusion has enough material to reorder. The final result is trimmed to limit.
const overscan = 20

// hybridSearch runs vector and keyword retrieval, fuses the scores, and returns
// the top results as memories.
func (s *Service) hybridSearch(ctx context.Context, h store.Hybrid, q store.Query, limit int) ([]memory.Memory, error) {
	wide := q
	wide.Limit = overscan

	vec, err := h.VectorSearch(ctx, wide)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	kw, err := h.KeywordSearch(ctx, wide)
	if err != nil {
		return nil, fmt.Errorf("keyword search: %w", err)
	}

	byID := map[string]memory.Memory{}
	toCandidates := func(scored []store.Scored) []ranking.Candidate {
		cs := make([]ranking.Candidate, 0, len(scored))
		for _, sc := range scored {
			byID[sc.Memory.ID] = sc.Memory
			cs = append(cs, ranking.Candidate{ID: sc.Memory.ID, Score: sc.Score})
		}
		return cs
	}

	fused := ranking.Fuse(toCandidates(vec), toCandidates(kw), ranking.DefaultWeights())
	if limit <= 0 || limit > len(fused) {
		limit = len(fused)
	}
	out := make([]memory.Memory, 0, limit)
	for _, c := range fused[:limit] {
		out = append(out, byID[c.ID])
	}
	return out, nil
}

// Delete removes a memory by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.store.Delete(ctx, id)
}

// List enumerates stored memories for a workspace. It returns an error if the
// underlying store does not support listing.
func (s *Service) List(ctx context.Context, workspaceID string) ([]memory.Memory, error) {
	lister, ok := s.store.(store.Lister)
	if !ok {
		return nil, fmt.Errorf("store %T does not support listing", s.store)
	}
	return lister.List(ctx, workspaceID)
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
