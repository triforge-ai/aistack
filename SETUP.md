# Installing `ai` (Personal AI Operating System)

This guide takes you from zero to a working setup. There are three configuration
tiers — pick the one that fits; you don't have to do them all.

| Tier | Embedder | Store | Extra requirements | Use when |
| --- | --- | --- | --- | --- |
| 1. Minimal | hash | file (JSON) | nothing | quick trial, fully offline |
| 2. Local semantic | Ollama | file (JSON) | Ollama | real semantic search, still local |
| 3. Production | Ollama | pgvector | Ollama + Docker | hybrid search, large datasets |

> Philosophy: everything is a swappable backend behind an interface. Moving up a
> tier is just **changing a few config lines**, not a rewrite.

---

## 0. Requirements

| Component | Required? | Notes |
| --- | --- | --- |
| Go ≥ 1.22 | ✅ | to build the binary |
| Docker + Docker Compose | ⬜ | tier 3 only (pgvector) |
| Ollama | ⬜ | tiers 2–3 only (local embeddings) |
| claude / cursor-agent / agy | ⬜ | only if you want to run real agents |

Quick check:

```bash
go version            # need go1.22+
docker version        # optional
ollama --version      # optional
```

---

## 1. Build

```bash
cd ai-operation
make build            # produces ./bin/ai
./bin/ai version
```

> **macOS note:** on Darwin with an older Go, the internal linker produces a
> binary missing `LC_UUID`, which dyld refuses to run (`Killed: 9` / abort).
> `make build` already handles this (external linker + ad-hoc codesign). **Always
> use `make build` / `make test`** instead of calling `go build` directly.
> Upgrading Go also fixes the issue.

For convenience, add `bin/` to your PATH or create an alias:

```bash
export PATH="$PWD/bin:$PATH"     # add to ~/.zshrc to make it permanent
# or:
alias ai="$PWD/bin/ai"
```

The sections below assume the `ai` command is available.

---

## 2. Tier 1 — Minimal (zero-config)

No database, no API key, works immediately.

```bash
mkdir my-workspace && cd my-workspace
ai init                 # create the .ai/ directory
ai status               # inspect the workspace
```

Add and search memory:

```bash
ai memory add "The project uses Go + pgvector for semantic memory"
ai memory sync          # index files under .ai/documents/
ai memory search "vector database"
ai memory list
```

Inspect the prompt the Context Builder produces (the default provider is
`dryrun` — it only prints, it does not call a model):

```bash
ai context backend "design the storage layer"
ai run backend "design the storage layer"      # also dryrun until you change the provider
```

Memory is persisted durably in `.ai/.cache/memory.json` (kept across runs).

---

## 3. Tier 2 — Semantic search with Ollama (still local)

The `hash` embedder only matches keywords. For **semantic** search, use Ollama —
still fully offline.

### 3.1 Install and run Ollama

```bash
# macOS
brew install ollama         # or download from https://ollama.com
ollama serve &              # run the server in the background (localhost:11434)
ollama pull nomic-embed-text
```

Verify:

```bash
curl -s http://localhost:11434/api/tags | grep nomic-embed-text
```

### 3.2 Declare it in the workspace

Edit `.ai/workspace.yaml`:

```yaml
id: my-workspace

embedder:
  type: ollama
  model: nomic-embed-text
  dimension: 768            # must match the model exactly
  host: http://localhost:11434
  cache: true               # reuse vectors for identical text (on by default)
```

### 3.3 Re-index with the new embedder

Changing the embedder changes the vector space, so you must re-index:

```bash
rm .ai/.cache/memory.json .ai/.cache/sync-*.json   # drop the old (hash) index
ai memory sync
ai memory add "semantic retrieval combines embeddings with keywords"
ai memory search "how does meaning-based search work"
```

Results are now ranked by meaning, not just literal matches. Vectors are cached
in `.ai/.cache/embed-cache.json`, so later runs don't re-embed.

---

## 4. Tier 3 — Production with pgvector + hybrid search

Add Postgres + pgvector for **hybrid search** (vector HNSW + BM25 keyword,
merged with Reciprocal Rank Fusion).

### 4.1 Start the database

`docker-compose.yml` is already provided (image `pgvector/pgvector:pg16`).

```bash
# run from the repo root (where docker-compose.yml lives)
make db-up          # = docker compose up -d
```

If port 5432 is busy, change `ports` in `docker-compose.yml` (e.g. `5433:5432`)
and adjust `port` in the config below to match.

### 4.2 Declare storage

Add to `.ai/workspace.yaml`:

```yaml
storage:
  type: pgvector
  host: localhost
  port: 5432
  user: ai
  password: ai
  db: ai_workspace
```

### 4.3 Initialize & sync

```bash
ai db ping          # check the connection + auto-run migrations (table, HNSW, GIN)
ai status           # storage: pgvector
ai memory sync      # now writes straight into Postgres
```

Try hybrid search (exact identifier + semantics):

```bash
ai memory add "ADR-014 records the decision to use reciprocal rank fusion"
ai memory search ADR-014                          # BM25 finds the exact identifier
ai memory search "how to combine multiple ranking signals"   # vector finds the meaning
```

### 4.4 Managing the DB

```bash
ai db status        # docker compose ps
ai db logs          # view logs
ai db down          # stop (data remains in the pgdata volume)
```

> ⚠️ **Embedding dimension**: HNSW needs vectors of a fixed dimension, so the
> system locks the column dimension to the embedder's dimension (768 for nomic).
> **Changing the model/dimension after you already have data** requires
> recreating the table. `ai` reports a clear error on a mismatch. To fix:
> `docker exec ai-postgres psql -U ai -d ai_workspace -c "DROP TABLE memory;"`
> then `ai memory sync` again.

---

## 5. Agent CLI integration (claude / cursor / agy)

Providers are swappable runtimes. `claude`, `cursor` (`cursor-agent`), and `agy`
are built in.

```bash
ai providers        # list + check which CLIs are installed
```

Run once with a specific provider:

```bash
ai run backend "add caching to storage" --provider claude
ai run backend "refactor the handler"   --provider cursor
ai run backend "write tests"            --provider agy
```

Set a default or register your own CLI in `.ai/workspace.yaml`:

```yaml
default_provider: claude          # used when the agent doesn't specify one

providers:                        # add/override your own CLI
  - name: mycli
    bin: my-agent
    args: ["--print"]             # use "{{prompt}}" to insert the prompt; otherwise it's appended
    stdin: true                   # or pipe the prompt over stdin
```

Pin a provider to a specific agent — `.ai/agents/backend.yaml`:

```yaml
name: backend
provider: cursor
system: You are a senior backend engineer. Precise and pragmatic.
rules:
  - coding
skills:
  - review
```

**Provider resolution order:** `--provider` (flag) → `provider:` in the agent →
`default_provider` → `dryrun`.

> ⚠️ **Safety:** the default is `dryrun` (prints the prompt only). `cursor`/`claude`
> can **write files and run shell commands**, so a real agent only runs when you
> deliberately choose it. Make sure those CLIs are logged in / configured with an
> API key per their own instructions.

---

## 6. The `.ai/` workspace layout

```
.ai/
├── workspace.yaml      # id + config (embedder, storage, providers)
├── rules/*.md          # always-on rules, loaded straight into the prompt
├── skills/*.md         # reusable skills
├── agents/*.yaml       # agent definitions (provider, rules, skills, system)
├── documents/*.md      # documents indexed for semantic search
├── tasks/
├── memory/
└── .cache/             # derived data (memory.json, embed-cache, sync state)
                        #   → safe to delete, already .gitignored; rebuilt by sync
```

`.ai/` + Obsidian is the **source of truth**; pgvector is only the semantic index
layer.

Sync an external Obsidian vault (optional) — in `workspace.yaml`:

```yaml
obsidian: $HOME/Obsidian/MyVault
```

```bash
ai memory sync obsidian      # or `ai memory sync` to sync both documents/ + obsidian
```

Sync is **incremental** (only changed files are re-embedded — diffed by content hash).

---

## 7. Daily workflow

```bash
ai memory sync                              # update the index after editing docs
ai memory search "<keyword/question>"        # quick lookup
ai context <agent> "<task>"                 # inspect the context to be sent (debug, no model call)
ai run <agent> "<task>" --provider claude   # run for real
```

---

## 8. Troubleshooting

| Symptom | Cause & fix |
| --- | --- |
| `Killed: 9` / `missing LC_UUID` when running `ai` | Use `make build` (not `go build`); or upgrade Go. |
| `no .ai/ workspace found` | Run `ai init`, or `cd` into a directory containing `.ai/`. |
| `memory search` returns `(no results)` | You haven't run `ai memory sync` / `ai memory add`. |
| Ollama: `connection refused` | `ollama serve` isn't running, or the `host` is wrong. |
| Ollama: `returned dim X, config expects Y` | The `dimension` in config doesn't match the model. Fix it (nomic = 768). |
| pgvector: `expected N dimensions` | The existing table has a different dimension. `DROP TABLE memory` then `ai memory sync`. |
| `db up`: `port is already allocated` | Port 5432 is busy. Change `ports` in compose + `port` in config. |
| `ai providers` reports `no (not on PATH)` | The CLI isn't installed or isn't on PATH. |

---

## 9. Command quick reference

```text
ai init [dir]                  create a .ai/ workspace
ai status                      inspect the workspace (storage, embedder, agents)
ai run <agent> <task...>       run an agent  [--provider <name>] [--limit <n>]
ai context <agent> <task...>   print the assembled prompt only
ai memory add <text...>        add a note (or pipe via stdin)
ai memory search <query...>    semantic search  [--limit <n>]
ai memory list                 list memories
ai memory rm <id>              delete a memory
ai memory sync [name]          sync documents/ (+ obsidian) into memory
ai db <up|down|status|ping>    manage pgvector via docker compose
ai providers                   list providers + installation status
ai version
```

For architecture details, see [`README.md`](README.md), [`OVERVIEW.md`](OVERVIEW.md),
and [`MEMORY_CONTEXT_BUILDER.md`](MEMORY_CONTEXT_BUILDER.md).
