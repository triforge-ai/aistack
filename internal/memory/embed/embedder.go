// Package embed turns text into vectors. The interface is provider-agnostic so
// the OpenAI embedder, a local model, or the zero-dependency hash embedder are
// all interchangeable.
package embed

// Embedder converts text into a fixed-dimension embedding.
type Embedder interface {
	// ID uniquely identifies the embedder (model + dim). It namespaces the
	// embedding cache so vectors from different models never collide.
	ID() string
	// Dim reports the dimensionality of vectors this embedder produces.
	Dim() int
	// Embed returns the embedding for text.
	Embed(text string) ([]float32, error)
}
