// Package memory defines the core memory record shared across the system.
//
// A Memory is the atomic unit of retrievable knowledge. The canonical source
// of truth lives on disk (.ai/ + Obsidian); the embedding turns it into a row
// in the semantic retrieval index (pgvector or the in-memory store).
package memory

// Type classifies a memory by what it represents.
type Type string

const (
	TypeRule  Type = "rule"
	TypeSkill Type = "skill"
	TypeDoc   Type = "doc"
	TypeTask  Type = "task"
	TypeChat  Type = "chat"
	TypeNote  Type = "note"
)

// Source records where a memory originated.
type Source string

const (
	SourceObsidian Source = "obsidian"
	SourceCLI      Source = "cli"
	SourceAgent    Source = "agent"
)

// Memory is one retrievable unit of knowledge.
type Memory struct {
	ID          string
	WorkspaceID string

	Type   Type
	Source Source

	Content   string
	Embedding []float32

	Metadata map[string]any

	CreatedAt int64 // unix seconds
}
