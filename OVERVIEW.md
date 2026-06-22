# 1. Overall architecture (Go + pgvector)

```text
AI CLI (Go)
    │
    ▼
AI Workspace Engine
    ├── Memory Engine (pgvector)
    ├── Rules Engine
    ├── Skills Engine
    ├── Document Indexer (Obsidian)
    ├── Task System
    ├── Agent Runtime
    └── Context Builder
            │
            ▼
     Provider Runtime Layer
        ├── Claude CLI
        ├── Codex CLI
        ├── Gemini CLI
        ├── OpenAI API (future)
        └── Ollama (future)
```

---

# 2. Core principle (most important)

### ❌ The old, wrong direction:

```text
/claude do X
/codex do Y
/gemini do Z
```

→ vendor-driven workflow

---

### ✅ The new, right direction:

```text
ai run backend "do X"
ai run architect "design Y"
```

→ agent-driven workflow

The provider is only a runtime backend.

---

# 3. pgvector schema (workspace-centric)

## 3.1 memory table (core)

```sql
CREATE TABLE memory (
    id UUID PRIMARY KEY,
    workspace_id TEXT NOT NULL,

    type TEXT NOT NULL,  -- rule | skill | doc | task | chat | note
    source TEXT NOT NULL, -- obsidian | cli | agent

    content TEXT NOT NULL,
    embedding vector(1536),

    metadata JSONB DEFAULT '{}',

    created_at TIMESTAMP DEFAULT now()
);
```

---

## 3.2 optional: agent memory separation

```sql
CREATE TABLE agent_memory (
    id UUID PRIMARY KEY,
    workspace_id TEXT,
    agent TEXT,

    content TEXT,
    embedding vector(1536),

    metadata JSONB,
    created_at TIMESTAMP
);
```

---

## 3.3 index strategy

```sql
CREATE INDEX memory_embedding_idx
ON memory
USING hnsw (embedding vector_cosine_ops);
```

👉 HNSW is the best choice for an AI OS (low-latency retrieval).

---

# 4. Go architecture (production-grade)

## 4.1 project structure

```text
cmd/ai/

internal/
  workspace/
  memory/
    store/
    embed/
    chunk/
  context/
  agent/
  provider/
  rules/
  skills/
  tasks/
  obsidian/
  git/
```

---

# 5. Memory Engine (pgvector layer)

## 5.1 interface

```go
type MemoryStore interface {
    Save(ctx context.Context, m Memory) error

    Search(ctx context.Context, queryEmbedding []float32, limit int) ([]Memory, error)

    Delete(ctx context.Context, id string) error
}
```

---

## 5.2 memory model

```go
type Memory struct {
    ID          string
    WorkspaceID string
    Type        string
    Source      string
    Content     string
    Metadata    map[string]any
    Embedding   []float32
}
```

---

## 5.3 vector search

```go
rows, err := db.Query(`
SELECT content, metadata
FROM memory
WHERE workspace_id = $1
ORDER BY embedding <-> $2
LIMIT $3
`, workspaceID, embedding, 10)
```

---

# 6. Context Builder (core intelligence layer)

This is the "brain" of the system.

```text
input: task
↓
1. load rules
2. load skills
3. load relevant memory (pgvector)
4. load documents (obsidian index)
5. attach task context
↓
final prompt
```

---

## 6.1 Go model

```go
type Context struct {
    Rules    []string
    Skills   []string
    Memory   []Memory
    Docs     []Document
    Task     string
}
```

---

## 6.2 builder

```go
type ContextBuilder interface {
    Build(ctx context.Context, agent string, task string) (Context, error)
}
```

---

# 7. Agent system (decoupled from the provider)

```go
type Agent struct {
    Name     string
    Provider Provider
    Rules    []string
    Skills   []string
}
```

---

## flow

```text
ai run backend "implement storage"
    ↓
Agent: backend
    ↓
Context Builder
    ↓
Provider (Claude/Codex/Gemini)
```

---

# 8. Provider layer (replaceable runtime)

```go
type Provider interface {
    Name() string

    Ask(ctx context.Context, prompt string) (string, error)

    Stream(ctx context.Context, prompt string, out chan<- string) error
}
```

---

## implementations:

```text
provider/
  claude/
  codex/
  gemini/
  openai/
  ollama/
```

---

# 9. Workspace standard mapping (very important)

## `.ai/` becomes the source of truth

```text
.ai/
├── workspace.yaml
├── memory/
├── rules/
├── skills/
├── documents/
├── tasks/
└── agents/
```

---

## mapping into the DB

| File        | pgvector    |
| ----------- | ----------- |
| rules/*.md  | type=rule   |
| skills/*.md | type=skill  |
| memory/*.md | type=memory |
| obsidian    | type=doc    |
| tasks       | type=task   |

---

# 10. Command design (final clean version)

## workspace

```bash
ai init
ai status
ai doctor
```

---

## agent

```bash
ai run backend "implement storage"
ai run architect "design system"
```

---

## provider (optional override)

```bash
ai run backend --provider codex "task"
```

---

## memory

```bash
ai memory add
ai memory search
ai memory sync obsidian
```

---

## context debugging

```bash
ai context build
```

---

## tasks

```bash
ai task create
ai task run
ai task list
```

---

## git integration

```bash
ai review
ai commit
ai pr
```

---

# 11. Critical design upgrade (most important)

## 11.1 pgvector is NOT the primary storage

It is:

```text
Semantic retrieval layer
```

---

## 11.2 the source of truth is still:

```text
.ai/ + Obsidian
```

---

## 11.3 the DB is only an index layer

```text
Obsidian → canonical data
Postgres → semantic index
```

---

# 12. Recommended final architecture

```text
                AI CLI (Go)
                      │
      ┌───────────────┼────────────────┐
      │               │                │
Workspace        Context Engine     Task System
      │               │                │
      └───────┬───────┘                │
              │                        │
        Memory Engine (pgvector)      │
              │                        │
        Obsidian Indexer              │
              │                        │
        Provider Runtime Layer        │
      (Claude / Codex / Gemini)       │
```

---

> 🧠 **Personal AI Operating System**

In this system pgvector is NOT a secondary feature — it is the:

```text
Central nervous system (semantic memory)
```

---
