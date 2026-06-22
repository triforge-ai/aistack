package embed

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OllamaEmbedder produces embeddings via a local Ollama server (default model
// nomic-embed-text, 768 dims). It keeps the system fully local — no cloud or
// API key — behind the same Embedder interface as everything else.
type OllamaEmbedder struct {
	host   string
	model  string
	dim    int
	client *http.Client
}

// NewOllamaEmbedder configures an Ollama-backed embedder. host defaults to
// http://localhost:11434, model to nomic-embed-text, dim to 768.
func NewOllamaEmbedder(host, model string, dim int) *OllamaEmbedder {
	if host == "" {
		host = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	if dim <= 0 {
		dim = 768
	}
	return &OllamaEmbedder{
		host:   host,
		model:  model,
		dim:    dim,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *OllamaEmbedder) ID() string { return "ollama:" + e.model }

func (e *OllamaEmbedder) Dim() int { return e.dim }

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaResponse struct {
	Embedding []float32 `json:"embedding"`
	Error     string    `json:"error"`
}

// Embed calls POST /api/embeddings. It validates the returned vector against the
// configured dimension so a misconfigured model fails loudly rather than
// silently poisoning the store.
func (e *OllamaEmbedder) Embed(text string) ([]float32, error) {
	body, err := json.Marshal(ollamaRequest{Model: e.model, Prompt: text})
	if err != nil {
		return nil, err
	}
	resp, err := e.client.Post(e.host+"/api/embeddings", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama embeddings request: %w (is `ollama serve` running?)", err)
	}
	defer resp.Body.Close()

	var out ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}
	if out.Error != "" {
		return nil, fmt.Errorf("ollama: %s (model %q pulled?)", out.Error, e.model)
	}
	if len(out.Embedding) == 0 {
		return nil, fmt.Errorf("ollama returned an empty embedding for model %q", e.model)
	}
	if len(out.Embedding) != e.dim {
		return nil, fmt.Errorf("ollama model %q returned dim %d, config expects %d — fix embedder.dimension",
			e.model, len(out.Embedding), e.dim)
	}
	return out.Embedding, nil
}
