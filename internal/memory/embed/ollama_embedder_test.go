package embed

import (
	"net/http"
	"testing"
	"time"
)

// TestOllamaEmbedderLive hits a local Ollama server. It self-skips when one is
// not reachable, so it is safe to run anywhere.
func TestOllamaEmbedderLive(t *testing.T) {
	host := "http://localhost:11434"
	client := &http.Client{Timeout: time.Second}
	if _, err := client.Get(host + "/api/tags"); err != nil {
		t.Skipf("no Ollama server at %s: %v", host, err)
	}

	e := NewOllamaEmbedder(host, "nomic-embed-text", 768)
	v, err := e.Embed("the quick brown fox")
	if err != nil {
		t.Skipf("ollama embed failed (model pulled?): %v", err)
	}
	if len(v) != 768 {
		t.Fatalf("expected 768 dims, got %d", len(v))
	}

	// Determinism: same text → same vector.
	v2, err := e.Embed("the quick brown fox")
	if err != nil {
		t.Fatal(err)
	}
	for i := range v {
		if v[i] != v2[i] {
			t.Fatalf("embedding not deterministic at index %d", i)
		}
	}
}
