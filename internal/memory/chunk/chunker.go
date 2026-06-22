// Package chunk splits documents into embeddable pieces. The current splitter
// is paragraph-aware with a rune-count cap; semantic / markdown-aware splitting
// is a planned upgrade.
package chunk

import "strings"

// Options controls chunking behaviour.
type Options struct {
	// MaxRunes caps the size of a single chunk. Paragraphs longer than this
	// are hard-split.
	MaxRunes int
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options { return Options{MaxRunes: 1200} }

// Chunk splits text on blank lines (paragraphs), then packs paragraphs into
// chunks up to opts.MaxRunes. Empty input yields no chunks.
func Chunk(text string, opts Options) []string {
	if opts.MaxRunes <= 0 {
		opts = DefaultOptions()
	}

	var chunks []string
	var cur strings.Builder
	curLen := 0

	flush := func() {
		if curLen > 0 {
			chunks = append(chunks, strings.TrimSpace(cur.String()))
			cur.Reset()
			curLen = 0
		}
	}

	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		for _, piece := range hardSplit(para, opts.MaxRunes) {
			n := len([]rune(piece))
			if curLen+n > opts.MaxRunes {
				flush()
			}
			if curLen > 0 {
				cur.WriteString("\n\n")
			}
			cur.WriteString(piece)
			curLen += n
		}
	}
	flush()
	return chunks
}

// hardSplit breaks a single oversized paragraph into rune-bounded pieces.
func hardSplit(para string, max int) []string {
	runes := []rune(para)
	if len(runes) <= max {
		return []string{para}
	}
	var out []string
	for i := 0; i < len(runes); i += max {
		end := i + max
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[i:end]))
	}
	return out
}
