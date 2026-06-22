package embed

import (
	"path/filepath"
	"testing"
)

// countingEmbedder records how many times Embed is actually invoked.
type countingEmbedder struct {
	calls int
}

func (c *countingEmbedder) ID() string { return "counting-4" }
func (c *countingEmbedder) Dim() int   { return 4 }
func (c *countingEmbedder) Embed(text string) ([]float32, error) {
	c.calls++
	return []float32{1, 2, 3, 4}, nil
}

func TestCacheAvoidsReEmbedding(t *testing.T) {
	inner := &countingEmbedder{}
	path := filepath.Join(t.TempDir(), "embed-cache.json")

	e, err := NewCachedEmbedder(inner, path)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if _, err := e.Embed("same text"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.Embed("other text"); err != nil {
		t.Fatal(err)
	}
	if inner.calls != 2 {
		t.Fatalf("expected 2 inner calls (one per unique text), got %d", inner.calls)
	}

	// Reopen: cache should persist, so no further inner calls for known text.
	e2, err := NewCachedEmbedder(inner, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e2.Embed("same text"); err != nil {
		t.Fatal(err)
	}
	if inner.calls != 2 {
		t.Fatalf("cache did not persist across reopen: inner calls = %d", inner.calls)
	}
}

func TestCacheKeyIncludesEmbedderID(t *testing.T) {
	a := &countingEmbedder{}
	ca, _ := NewCachedEmbedder(a, "")
	kA := ca.key("hello")

	hash := NewHashEmbedder(4)
	ch, _ := NewCachedEmbedder(hash, "")
	kH := ch.key("hello")

	if kA == kH {
		t.Fatal("different embedders must produce different cache keys for the same text")
	}
}
