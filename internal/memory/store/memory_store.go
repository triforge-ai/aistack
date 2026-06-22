package store

import (
	"context"
	"sync"

	"ai-cli/internal/memory"
)

// MemoryStore is an in-process Store used for tests and ephemeral runs. It lets
// the whole pipeline run without any persistence; it is not durable across
// restarts. For a durable dev backend see FileStore.
type MemoryStore struct {
	mu   sync.RWMutex
	rows map[string]memory.Memory
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{rows: make(map[string]memory.Memory)}
}

func (s *MemoryStore) Save(_ context.Context, m memory.Memory) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[m.ID] = m
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, id)
	return nil
}

// Search returns the top-N memories in the workspace ranked by cosine
// similarity to the query embedding.
func (s *MemoryStore) Search(_ context.Context, q Query) ([]memory.Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return rank(s.values(), q), nil
}

// List returns all memories in the workspace (all workspaces if id == "").
func (s *MemoryStore) List(_ context.Context, workspaceID string) ([]memory.Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return filterWorkspace(s.values(), workspaceID), nil
}

func (s *MemoryStore) values() []memory.Memory {
	out := make([]memory.Memory, 0, len(s.rows))
	for _, m := range s.rows {
		out = append(out, m)
	}
	return out
}
