// Package workspace loads the .ai/ directory, which is the canonical source of
// truth for a workspace: its config, rules, skills, documents, tasks and agent
// definitions. pgvector only ever mirrors what lives here.
package workspace

import "path/filepath"

// Workspace is the in-memory view of a loaded .ai/ directory.
type Workspace struct {
	ID   string
	Root string // absolute path to the .ai/ directory

	// Obsidian is an optional absolute path to an external vault to sync. When
	// empty, sync falls back to the workspace's documents/ directory.
	Obsidian string

	// Storage selects the memory backend (file by default, or pgvector).
	Storage StorageConfig

	// Embedder selects the embedding backend (hash by default, or ollama).
	Embedder EmbedderConfig

	// Chat tunes the interactive REPL (e.g. whether turns are persisted).
	Chat ChatConfig

	// Providers are extra/override agent CLI backends; DefaultProvider names the
	// one used when an agent does not specify its own.
	Providers       []ProviderConfig
	DefaultProvider string

	Rules  []Doc
	Skills []Doc
	Agents map[string]AgentDef
}

// SaveChatMemory reports whether chat turns should be persisted back into
// memory. It defaults to true (the config field is nil unless explicitly set).
func (w *Workspace) SaveChatMemory() bool {
	if w.Chat.SaveMemory != nil {
		return *w.Chat.SaveMemory
	}
	return true
}

// DocumentsDir returns the in-workspace documents directory.
func (w *Workspace) DocumentsDir() string {
	return filepath.Join(w.Root, "documents")
}

// SessionsDir returns the directory where chat sessions are persisted. Sessions
// are canonical conversation records (not derived), so they live alongside the
// other source-of-truth dirs, not under .cache/.
func (w *Workspace) SessionsDir() string {
	return filepath.Join(w.Root, "sessions")
}

// RecipesDir returns the directory holding declarative workflow recipes.
func (w *Workspace) RecipesDir() string {
	return filepath.Join(w.Root, "recipes")
}

// CacheDir returns the workspace's derived-data directory (memory store, sync
// state). It is safe to delete; it is rebuilt from canonical sources.
func (w *Workspace) CacheDir() string {
	return filepath.Join(w.Root, ".cache")
}

// Doc is a markdown file loaded from the workspace.
type Doc struct {
	Name    string // file name without extension
	Path    string
	Content string
}

// AgentDef is the declarative definition of an agent, loaded from
// .ai/agents/<name>.yaml.
type AgentDef struct {
	Name     string   `yaml:"name"`
	Provider string   `yaml:"provider"`
	Rules    []string `yaml:"rules"`
	Skills   []string `yaml:"skills"`
	System   string   `yaml:"system"`
}
