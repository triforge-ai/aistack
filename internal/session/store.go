package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ErrNotFound is returned by Load when no session has the given id.
var ErrNotFound = errors.New("session not found")

// Store persists and retrieves sessions.
type Store interface {
	Save(ctx context.Context, r Record) error
	Load(ctx context.Context, id string) (Record, error)
	// List returns the workspace's sessions, most-recently-updated first.
	List(ctx context.Context, workspace string) ([]Record, error)
	Delete(ctx context.Context, id string) error
}

// FileStore keeps one JSON file per session under a directory (one document per
// session, so list/load/delete stay independent). Writes are atomic.
type FileStore struct {
	mu  sync.Mutex
	dir string
}

// NewFileStore returns a session store rooted at dir (e.g. .ai/sessions).
func NewFileStore(dir string) *FileStore {
	return &FileStore{dir: dir}
}

// path returns the on-disk file for id, rejecting ids that could escape dir.
func (s *FileStore) path(id string) (string, error) {
	if id == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid session id %q", id)
	}
	return filepath.Join(s.dir, id+".json"), nil
}

func (s *FileStore) Save(_ context.Context, r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := s.path(r.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (s *FileStore) Load(_ context.Context, id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadFile(id)
}

func (s *FileStore) loadFile(id string) (Record, error) {
	p, err := s.path(id)
	if err != nil {
		return Record{}, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return Record{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return Record{}, err
	}
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return Record{}, fmt.Errorf("corrupt session %s: %w", p, err)
	}
	return r, nil
}

func (s *FileStore) List(_ context.Context, workspace string) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var out []Record
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		r, err := s.loadFile(id)
		if err != nil {
			return nil, err
		}
		if workspace != "" && r.Workspace != workspace {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

func (s *FileStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return err
	}
	return nil
}
