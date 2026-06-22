package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/triforge-ai/aistack/internal/memory"
)

// FileStore is a durable, single-file JSON Store. It is the default backend for
// the CLI: it survives across invocations (so `memory add` and `sync` persist)
// without requiring a database. It carries no retrieval intelligence — ranking
// is shared via rank() — and is meant to be swapped for PgVectorStore in
// production behind the same Store interface.
//
// The whole dataset is held in memory and rewritten on each mutation. That is
// fine for a personal workspace; it is explicitly not built for scale.
type FileStore struct {
	mu   sync.Mutex
	path string
	rows map[string]memory.Memory
}

// NewFileStore opens (or creates) a store backed by the JSON file at path.
func NewFileStore(path string) (*FileStore, error) {
	s := &FileStore{path: path, rows: map[string]memory.Memory{}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *FileStore) load() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var rows []memory.Memory
	if err := json.Unmarshal(data, &rows); err != nil {
		return fmt.Errorf("corrupt memory store %s: %w", s.path, err)
	}
	for _, m := range rows {
		s.rows[m.ID] = m
	}
	return nil
}

// flush writes the current rows atomically (temp file + rename).
func (s *FileStore) flush() error {
	rows := make([]memory.Memory, 0, len(s.rows))
	for _, m := range s.rows {
		rows = append(rows, m)
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *FileStore) Save(_ context.Context, m memory.Memory) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[m.ID] = m
	return s.flush()
}

func (s *FileStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, id)
	return s.flush()
}

func (s *FileStore) Search(_ context.Context, q Query) ([]memory.Memory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return rank(s.values(), q), nil
}

func (s *FileStore) List(_ context.Context, workspaceID string) ([]memory.Memory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return filterWorkspace(s.values(), workspaceID), nil
}

func (s *FileStore) values() []memory.Memory {
	out := make([]memory.Memory, 0, len(s.rows))
	for _, m := range s.rows {
		out = append(out, m)
	}
	return out
}
