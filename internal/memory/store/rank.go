package store

import (
	"sort"

	"github.com/triforge-ai/aistack/internal/memory"
	"github.com/triforge-ai/aistack/internal/vector"
)

// rank filters candidates to the queried workspace, orders them by cosine
// similarity to the query embedding, and applies the limit. This is the only
// "search logic" a Store needs; the ranking strategy lives here, not in any
// particular backend.
func rank(candidates []memory.Memory, q Query) []memory.Memory {
	type scored struct {
		m     memory.Memory
		score float32
	}

	var hits []scored
	for _, m := range candidates {
		if q.WorkspaceID != "" && m.WorkspaceID != q.WorkspaceID {
			continue
		}
		hits = append(hits, scored{m: m, score: vector.Cosine(q.Embedding, m.Embedding)})
	}

	sort.Slice(hits, func(i, j int) bool { return hits[i].score > hits[j].score })

	limit := q.Limit
	if limit <= 0 || limit > len(hits) {
		limit = len(hits)
	}

	out := make([]memory.Memory, 0, limit)
	for _, h := range hits[:limit] {
		out = append(out, h.m)
	}
	return out
}

// filterWorkspace returns memories in the given workspace (all if id == "").
func filterWorkspace(candidates []memory.Memory, workspaceID string) []memory.Memory {
	if workspaceID == "" {
		return candidates
	}
	var out []memory.Memory
	for _, m := range candidates {
		if m.WorkspaceID == workspaceID {
			out = append(out, m)
		}
	}
	return out
}
