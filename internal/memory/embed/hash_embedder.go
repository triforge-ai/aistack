package embed

import (
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

// HashEmbedder is a deterministic, dependency-free embedder. It maps tokens
// into a fixed-size bag-of-words vector via feature hashing and L2-normalises
// the result. It needs no API key or network, which makes it the default for
// local development and tests. Quality is modest; swap in OpenAIEmbedder for
// production retrieval.
type HashEmbedder struct {
	dim int
}

// NewHashEmbedder returns a HashEmbedder producing vectors of size dim.
func NewHashEmbedder(dim int) *HashEmbedder {
	if dim <= 0 {
		dim = 256
	}
	return &HashEmbedder{dim: dim}
}

func (e *HashEmbedder) ID() string { return fmt.Sprintf("hash-%d", e.dim) }

func (e *HashEmbedder) Dim() int { return e.dim }

func (e *HashEmbedder) Embed(text string) ([]float32, error) {
	vec := make([]float32, e.dim)
	for _, tok := range tokenize(text) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(tok))
		vec[h.Sum32()%uint32(e.dim)]++
	}

	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm > 0 {
		n := float32(math.Sqrt(norm))
		for i := range vec {
			vec[i] /= n
		}
	}
	return vec, nil
}

func tokenize(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}
