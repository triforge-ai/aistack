package embed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// CachedEmbedder wraps an Embedder with a content-addressed cache so identical
// text is embedded only once. The cache key includes the inner embedder's ID,
// so switching models never returns stale vectors. This matters a lot for the
// Ollama backend, where each call is a real model inference.
type CachedEmbedder struct {
	inner Embedder
	cache *fileCache
}

// NewCachedEmbedder wraps inner with a cache persisted at path. A path of ""
// yields an in-memory-only cache.
func NewCachedEmbedder(inner Embedder, path string) (*CachedEmbedder, error) {
	c, err := newFileCache(path)
	if err != nil {
		return nil, err
	}
	return &CachedEmbedder{inner: inner, cache: c}, nil
}

func (e *CachedEmbedder) ID() string { return e.inner.ID() }
func (e *CachedEmbedder) Dim() int   { return e.inner.Dim() }

func (e *CachedEmbedder) Embed(text string) ([]float32, error) {
	key := e.key(text)
	if v, ok := e.cache.get(key); ok {
		return v, nil
	}
	v, err := e.inner.Embed(text)
	if err != nil {
		return nil, err
	}
	if err := e.cache.put(key, v); err != nil {
		return nil, err
	}
	return v, nil
}

func (e *CachedEmbedder) key(text string) string {
	h := sha256.New()
	h.Write([]byte(e.inner.ID()))
	h.Write([]byte{0})
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}

// fileCache is a small JSON-backed map of key → vector. It loads once and
// flushes on each new entry; fine for a personal workspace.
type fileCache struct {
	mu      sync.Mutex
	path    string
	entries map[string][]float32
}

func newFileCache(path string) (*fileCache, error) {
	c := &fileCache{path: path, entries: map[string][]float32{}}
	if path == "" {
		return c, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	// A corrupt cache is non-fatal: drop it and rebuild.
	_ = json.Unmarshal(data, &c.entries)
	if c.entries == nil {
		c.entries = map[string][]float32{}
	}
	return c, nil
}

func (c *fileCache) get(key string) ([]float32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.entries[key]
	return v, ok
}

func (c *fileCache) put(key string, v []float32) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = v
	if c.path == "" {
		return nil
	}
	data, err := json.Marshal(c.entries)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}
