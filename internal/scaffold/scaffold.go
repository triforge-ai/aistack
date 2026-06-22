// Package scaffold creates a fresh .ai/ workspace on disk.
package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
)

// file is one scaffolded file: a path relative to .ai/ and its contents.
type file struct {
	rel     string
	content string
}

var files = []file{
	{"workspace.yaml", `id: my-workspace

# obsidian: $HOME/Obsidian/MyVault   # optional external vault to sync

# Embedding backend. Default is the zero-dependency hash embedder.
# For fully-local model embeddings, run Ollama and uncomment:
# embedder:
#   type: ollama
#   model: nomic-embed-text
#   dimension: 768
#   host: http://localhost:11434
#   cache: true

# Memory store. Default is pgvector — start it with ` + "`ai db up`" + ` (connection
# defaults to localhost:5432 ai/ai ai_workspace; override the fields below).
# storage:
#   type: pgvector
#   host: localhost
#   port: 5432
#   user: ai
#   password: ai
#   db: ai_workspace
#
# To run fully offline with no database, use the durable file backend instead:
# storage:
#   type: file

# Agent CLI providers. claude / cursor / agy are built in; uncomment to add
# your own or override. default_provider is used when an agent omits one.
# default_provider: claude
# providers:
#   - name: mycli
#     bin: my-agent
#     args: ["--print"]
#     stdin: true
`},
	{".gitignore", ".cache/\n"},
	{"rules/coding.md", "# Coding rules\n\n- Write small, composable functions.\n- Prefer clarity over cleverness.\n- Every exported symbol has a doc comment.\n"},
	{"skills/review.md", "# Skill: code review\n\nReview diffs for correctness, then for simplification and reuse.\n"},
	{"agents/backend.yaml", "name: backend\nprovider: dryrun\nsystem: You are a senior backend engineer. Be precise and pragmatic.\nrules:\n  - coding\nskills:\n  - review\n"},
	{"documents/getting-started.md", "# Getting started\n\nThis workspace stores documents here. Run `ai memory sync` to index them,\nthen `ai memory search` or `ai run` to retrieve them semantically.\n"},
	{"recipes/summarize-changes.yaml", `# A sample workflow. Run with: ai recipe run summarize-changes --allow-shell
name: summarize-changes
steps:
  - id: diff
    run: shell
    cmd: git diff --stat HEAD~1 2>/dev/null || echo "no previous commit"
  - id: summary
    run: agent
    agent: backend
    prompt: |
      Summarize these repository changes for a changelog:
      {{ step "diff" }}
`},
	{"tasks/.gitkeep", ""},
	{"memory/.gitkeep", ""},
}

// Init creates a .ai/ directory under dir. It refuses to overwrite an existing
// workspace. It returns the path to the created .ai/ directory.
func Init(dir string) (string, error) {
	root := filepath.Join(dir, ".ai")
	if _, err := os.Stat(root); err == nil {
		return "", fmt.Errorf(".ai/ already exists at %s", root)
	}

	for _, f := range files {
		dest := filepath.Join(root, f.rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(dest, []byte(f.content), 0o644); err != nil {
			return "", err
		}
	}
	return root, nil
}
