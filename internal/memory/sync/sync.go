// Package sync incrementally mirrors a directory of markdown files (e.g. an
// Obsidian vault or .ai/documents) into the memory engine.
//
// It is a logic-layer component: it decides *what* changed (content-hash diff),
// then drives chunk → embed → store through the memory service. It is agnostic
// to which Store backend sits underneath. State (path → hash → memory IDs) is
// persisted so re-syncs are incremental.
package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/triforge-ai/aistack/internal/memory"
	"github.com/triforge-ai/aistack/internal/memory/service"
)

// Source identifies a directory to sync and the logical name it is filed under.
type Source struct {
	Name string // e.g. "obsidian" or "documents"
	Dir  string // absolute path to scan (recursively) for *.md
}

// Report summarises a sync run.
type Report struct {
	Added     int
	Updated   int
	Removed   int
	Unchanged int
}

// Syncer drives incremental syncs.
type Syncer struct {
	mem      *service.Service
	stateDir string
}

// New returns a Syncer that persists per-source state under stateDir.
func New(mem *service.Service, stateDir string) *Syncer {
	return &Syncer{mem: mem, stateDir: stateDir}
}

// fileState tracks one source file's last-synced hash and the memory IDs it
// produced (a file may yield several chunks).
type fileState struct {
	Hash string   `json:"hash"`
	IDs  []string `json:"ids"`
}

type state struct {
	Files map[string]fileState `json:"files"`
}

// Sync reconciles the source directory with the memory store for the given
// workspace and returns a Report. Missing directories are treated as empty
// (which removes any previously-synced content).
func (s *Syncer) Sync(ctx context.Context, workspaceID string, src Source) (Report, error) {
	var rep Report

	st, err := s.loadState(src.Name)
	if err != nil {
		return rep, err
	}

	seen := map[string]bool{}
	walkErr := filepath.WalkDir(src.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(src.Dir, path)
		if err != nil {
			return err
		}
		seen[rel] = true

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hash := hashBytes(content)

		prev, existed := st.Files[rel]
		if existed && prev.Hash == hash {
			rep.Unchanged++
			return nil
		}
		if existed {
			// Changed: drop the stale chunks before re-adding.
			if err := s.deleteIDs(ctx, prev.IDs); err != nil {
				return err
			}
			rep.Updated++
		} else {
			rep.Added++
		}

		ids, err := s.mem.Add(ctx, service.AddInput{
			WorkspaceID: workspaceID,
			Type:        memory.TypeDoc,
			Source:      memory.SourceObsidian,
			Content:     string(content),
			Metadata:    map[string]any{"name": rel, "path": path, "source": src.Name},
		})
		if err != nil {
			return err
		}
		st.Files[rel] = fileState{Hash: hash, IDs: ids}
		return nil
	})
	if walkErr != nil {
		return rep, walkErr
	}

	// Files gone from disk: remove their memories.
	for rel, fs := range st.Files {
		if seen[rel] {
			continue
		}
		if err := s.deleteIDs(ctx, fs.IDs); err != nil {
			return rep, err
		}
		delete(st.Files, rel)
		rep.Removed++
	}

	if err := s.saveState(src.Name, st); err != nil {
		return rep, err
	}
	return rep, nil
}

func (s *Syncer) deleteIDs(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if err := s.mem.Delete(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Syncer) statePath(name string) string {
	return filepath.Join(s.stateDir, "sync-"+name+".json")
}

func (s *Syncer) loadState(name string) (state, error) {
	st := state{Files: map[string]fileState{}}
	data, err := os.ReadFile(s.statePath(name))
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, fmt.Errorf("corrupt sync state %s: %w", s.statePath(name), err)
	}
	if st.Files == nil {
		st.Files = map[string]fileState{}
	}
	return st, nil
}

func (s *Syncer) saveState(name string, st state) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.stateDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.statePath(name), data, 0o644)
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
