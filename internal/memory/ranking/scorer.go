// Package ranking fuses results from multiple retrievers (vector + keyword)
// into a single ordering. It is deliberately pure — it knows nothing about
// stores, embeddings, or SQL — so the fusion policy is unit-testable in
// isolation and is the same regardless of backend.
package ranking

import "sort"

// Candidate is one retriever's opinion about an item: an ID and a raw score.
type Candidate struct {
	ID    string
	Score float64
}

// Weights control how much each retriever contributes to the fused score.
type Weights struct {
	Vector  float64
	Keyword float64
}

// DefaultWeights favours semantic relevance while letting exact keyword hits
// (identifiers like "ADR-001", "bug-123") surface.
func DefaultWeights() Weights { return Weights{Vector: 0.7, Keyword: 0.3} }

// rrfK dampens the influence of the very top ranks. 60 is the value from the
// original Reciprocal Rank Fusion paper and the common default.
const rrfK = 60.0

// Fuse combines vector and keyword candidates using weighted Reciprocal Rank
// Fusion. Each list must already be ordered best-first; an item's contribution
// is w / (k + rank), so fusion depends only on rank position — not on the
// incomparable raw scales of cosine similarity and ts_rank. Items appearing in
// both lists accumulate from both, which is exactly the hybrid signal we want.
// The result is sorted by fused score, descending.
func Fuse(vector, keyword []Candidate, w Weights) []Candidate {
	fused := map[string]float64{}
	accumulate := func(cs []Candidate, weight float64) {
		for rank, c := range cs {
			fused[c.ID] += weight / (rrfK + float64(rank+1))
		}
	}
	accumulate(vector, w.Vector)
	accumulate(keyword, w.Keyword)

	out := make([]Candidate, 0, len(fused))
	for id, s := range fused {
		out = append(out, Candidate{ID: id, Score: s})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID // stable, deterministic tie-break
	})
	return out
}
