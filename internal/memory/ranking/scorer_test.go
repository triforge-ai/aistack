package ranking

import "testing"

func ids(cs []Candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func TestFusePrefersItemsInBothLists(t *testing.T) {
	// "b" is mid-ranked in each list but appears in both, so RRF should lift it
	// above items that are strong in only one retriever.
	vector := []Candidate{{"a", 1.0}, {"b", 0.6}}
	keyword := []Candidate{{"c", 1.0}, {"b", 0.6}}

	got := Fuse(vector, keyword, Weights{Vector: 0.5, Keyword: 0.5})
	if got[0].ID != "b" {
		t.Fatalf("expected 'b' (present in both) ranked first, got %v", ids(got))
	}
}

func TestFuseKeywordOnlyStillSurfaces(t *testing.T) {
	// An exact keyword hit absent from vector results must still appear.
	vector := []Candidate{{"a", 1.0}}
	keyword := []Candidate{{"ADR-001", 1.0}}

	got := Fuse(vector, keyword, DefaultWeights())
	found := false
	for _, c := range got {
		if c.ID == "ADR-001" {
			found = true
		}
	}
	if !found {
		t.Fatalf("keyword-only hit dropped: %v", ids(got))
	}
}

func TestFuseEmptyInputs(t *testing.T) {
	if got := Fuse(nil, nil, DefaultWeights()); len(got) != 0 {
		t.Fatalf("expected empty, got %v", ids(got))
	}
}

func TestFuseRankNotRawScore(t *testing.T) {
	// RRF depends on rank position, not raw score magnitude. A list with tiny
	// raw scores still contributes fully based on order.
	got := Fuse([]Candidate{{"a", 0.001}, {"b", 0.0005}}, nil, Weights{Vector: 1, Keyword: 0})
	if len(got) != 2 || got[0].ID != "a" {
		t.Fatalf("expected rank order a,b regardless of magnitude: %v", ids(got))
	}
	want := 1.0 / (60.0 + 1.0)
	if got[0].Score != want {
		t.Fatalf("top RRF score = %v, want %v", got[0].Score, want)
	}
}
