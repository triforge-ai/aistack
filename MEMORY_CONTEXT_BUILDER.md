
Goal: **an agent-driven AI OS where the provider is only a runtime**

---

# 1. Module architecture overview

```text id="g7xk2p"
internal/
  memory/
    store/
      pgvector_store.go
    embed/
      embedder.go
      openai_embedder.go
    chunk/
      chunker.go
    service/
      memory_service.go

  context/
    builder.go
    assembler.go
    selector.go

  workspace/
    loader.go
```

---

# 2. Memory Engine design

## 2.1 Core model

```go id="m3x9qv"
package memory

type Memory struct {
    ID          string
    WorkspaceID string

    Type   string // rule | skill | doc | task | chat
    Source string // obsidian | cli | agent

    Content   string
    Embedding []float32

    Metadata map[string]any

    CreatedAt int64
}
```

---

# 2.2 Store interface (decouple DB)

```go id="q9k2mz"
package store

import (
    "context"
    "ai-cli/internal/memory"
)

type Store interface {
    Save(ctx context.Context, m memory.Memory) error

    Search(ctx context.Context, embedding []float32, limit int) ([]memory.Memory, error)

    Delete(ctx context.Context, id string) error
}
```

---

# 2.3 pgvector implementation

```go id="p7n4sv"
package store

import (
    "context"
    "database/sql"
    "ai-cli/internal/memory"
)

type PgVectorStore struct {
    db *sql.DB
}

func NewPgVectorStore(db *sql.DB) *PgVectorStore {
    return &PgVectorStore{db: db}
}
```

---

## Save

```go id="x8k3qp"
func (s *PgVectorStore) Save(ctx context.Context, m memory.Memory) error {
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO memory (
            id, workspace_id, type, source,
            content, embedding, metadata, created_at
        )
        VALUES ($1,$2,$3,$4,$5,$6,$7,to_timestamp($8))
    `,
        m.ID,
        m.WorkspaceID,
        m.Type,
        m.Source,
        m.Content,
        m.Embedding,
        m.Metadata,
        m.CreatedAt,
    )
    return err
}
```

---

## Vector search

```go id="c1v8fd"
func (s *PgVectorStore) Search(
    ctx context.Context,
    embedding []float32,
    limit int,
) ([]memory.Memory, error) {

    rows, err := s.db.QueryContext(ctx, `
        SELECT id, workspace_id, type, source, content, metadata
        FROM memory
        ORDER BY embedding <-> $1
        LIMIT $2
    `, embedding, limit)

    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var result []memory.Memory

    for rows.Next() {
        var m memory.Memory
        var meta map[string]any

        err := rows.Scan(
            &m.ID,
            &m.WorkspaceID,
            &m.Type,
            &m.Source,
            &m.Content,
            &meta,
        )
        if err != nil {
            return nil, err
        }

        m.Metadata = meta
        result = append(result, m)
    }

    return result, nil
}
```

---

# 3. Embedder layer

## 3.1 Interface

```go id="v5q9ld"
package embed

type Embedder interface {
    Embed(text string) ([]float32, error)
}
```

---

## 3.2 OpenAI implementation (default)

```go id="w9r3kc"
package embed

type OpenAIEmbedder struct {
    apiKey string
}

func (e *OpenAIEmbedder) Embed(text string) ([]float32, error) {
    // call embedding API
    return []float32{}, nil
}
```

---

## 3.3 (optional) cache embedding

```go id="l4x8qp"
var cache = map[string][]float32{}
```

👉 in production this is later replaced by Redis or a local hash

---

# 4. Chunker (critical for quality)

```go id="k2v7mz"
package chunk

func Chunk(text string, size int) []string {
    var chunks []string
    runes := []rune(text)

    for i := 0; i < len(runes); i += size {
        end := i + size
        if end > len(runes) {
            end = len(runes)
        }
        chunks = append(chunks, string(runes[i:end]))
    }

    return chunks
}
```

---

👉 later upgrades:

* semantic chunking (LLM-based split)
* markdown-aware splitting

---

# 5. Memory Service (core orchestration)

```go id="b9m3kp"
package service

import (
    "context"
    "ai-cli/internal/memory"
    "ai-cli/internal/memory/embed"
    "ai-cli/internal/memory/store"
)
```

---

## Service

```go id="z7n2qp"
type Service struct {
    store    store.Store
    embedder embed.Embedder
}
```

---

## Add memory

```go id="x4k8qp"
func (s *Service) Add(ctx context.Context, m memory.Memory) error {

    vec, err := s.embedder.Embed(m.Content)
    if err != nil {
        return err
    }

    m.Embedding = vec

    return s.store.Save(ctx, m)
}
```

---

## Search memory

```go id="r8v2mc"
func (s *Service) Search(
    ctx context.Context,
    query string,
    limit int,
) ([]memory.Memory, error) {

    vec, err := s.embedder.Embed(query)
    if err != nil {
        return nil, err
    }

    return s.store.Search(ctx, vec, limit)
}
```

---

# 6. Context Builder (CORE SYSTEM)

This is the "brain" of the AI OS.

---

## 6.1 Input model

```go id="t7p3qc"
package context

type BuildRequest struct {
    WorkspaceID string
    Agent       string
    Task        string
}
```

---

## 6.2 Context output

```go id="v9k2xn"
type Context struct {
    Rules  []string
    Skills []string
    Memory []string
    Task   string
}
```

---

# 6.3 Builder interface

```go id="c3x8pm"
type Builder interface {
    Build(ctx context.Context, req BuildRequest) (Context, error)
}
```

---

# 6.4 Implementation

```go id="m5q9zv"
type DefaultBuilder struct {
    memoryService *memory.Service
    loader        *workspace.Loader
}
```

---

## Build logic

```go id="u2n7qp"
func (b *DefaultBuilder) Build(
    ctx context.Context,
    req BuildRequest,
) (Context, error) {

    ws := b.loader.Load(req.WorkspaceID)

    memories, _ := b.memoryService.Search(ctx, req.Task, 5)

    return Context{
        Rules:  ws.Rules,
        Skills: ws.Skills,
        Task:   req.Task,
        Memory: extract(memories),
    }, nil
}
```

---

# 7. Prompt Assembler (final step)

```go id="y8v3qp"
func Assemble(c Context) string {

    return `
[RULES]
` + join(c.Rules) + `

[SKILLS]
` + join(c.Skills) + `

[MEMORY]
` + join(c.Memory) + `

[TASK]
` + c.Task
}
```

---

# 8. Full flow

```text id="n4x9qp"
ai run backend "implement storage"

↓
ContextBuilder.Build()

↓
Memory.Search(pgvector)

↓
Workspace rules + skills

↓
PromptAssembler

↓
Provider (Claude/Codex/Gemini)

↓
Response
```

---

# 9. Key design decisions (important)

## 9.1 pgvector is used only for:

* semantic memory
* retrieval
* context augmentation

❌ not used as primary storage

---

## 9.2 the workspace is the source of truth

```text id="q2v8mp"
.ai/ folder + Obsidian
```

---

## 9.3 the provider is fully replaceable

```text id="f8k2xp"
Claude ≠ logic
Codex ≠ logic
Gemini ≠ logic
```

---

## 9.4 the context builder is the "core intelligence"

If the system later fails to scale:

👉 the fault is in the context builder, not the DB

---
