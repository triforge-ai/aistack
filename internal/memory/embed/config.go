package embed

import "fmt"

// Kind selects an embedder backend.
type Kind string

const (
	KindHash   Kind = "hash"   // zero-dependency local default
	KindOllama Kind = "ollama" // local model server (e.g. nomic-embed-text)
)

// Config selects and parameterises an embedder.
type Config struct {
	Kind  Kind
	Model string
	// Dimension is the expected vector size. For hash it sets the size; for
	// ollama it validates the model's output.
	Dimension int
	Host      string
	// Cache enables the content-addressed embedding cache.
	Cache bool
	// CachePath is where the cache persists (empty → in-memory only).
	CachePath string
}

// New builds an embedder from cfg, wrapping it in a cache when enabled. Like the
// store factory, this is the single seam: MemoryService never learns which
// embedder is active.
func New(cfg Config) (Embedder, error) {
	var base Embedder
	switch cfg.Kind {
	case KindHash, "":
		dim := cfg.Dimension
		if dim <= 0 {
			dim = 256
		}
		base = NewHashEmbedder(dim)
	case KindOllama:
		base = NewOllamaEmbedder(cfg.Host, cfg.Model, cfg.Dimension)
	default:
		return nil, fmt.Errorf("unknown embedder kind %q", cfg.Kind)
	}

	if !cfg.Cache {
		return base, nil
	}
	return NewCachedEmbedder(base, cfg.CachePath)
}
