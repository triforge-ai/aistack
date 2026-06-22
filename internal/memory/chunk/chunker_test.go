package chunk

import (
	"strings"
	"testing"
)

func TestChunkEmpty(t *testing.T) {
	if got := Chunk("   \n\n  ", DefaultOptions()); len(got) != 0 {
		t.Fatalf("want no chunks, got %d", len(got))
	}
}

func TestChunkPacksParagraphs(t *testing.T) {
	text := "para one\n\npara two\n\npara three"
	got := Chunk(text, Options{MaxRunes: 1000})
	if len(got) != 1 {
		t.Fatalf("small paragraphs should pack into 1 chunk, got %d", len(got))
	}
}

func TestChunkHardSplitsOversized(t *testing.T) {
	long := strings.Repeat("x", 50)
	got := Chunk(long, Options{MaxRunes: 10})
	if len(got) != 5 {
		t.Fatalf("want 5 chunks, got %d", len(got))
	}
}
