# ai — Personal AI Operating System

An **agent-driven** AI workspace. Agents are the unit of work; providers
(Claude / Codex / Gemini) are interchangeable runtime backends.

```
ai run backend "implement storage"   ✅  agent-driven
/claude do X                          ❌  vendor-driven (the old way)
```

Design docs: [`OVERVIEW.md`](OVERVIEW.md), [`MEMORY_CONTEXT_BUILDER.md`](MEMORY_CONTEXT_BUILDER.md).

## Status — v0.4 (fully-local, offline-first)

The pipeline is **offline-first with no cloud or API-key dependency**. It runs
zero-config out of the box, and every external-feeling piece is a swappable
local backend behind a stable interface:

- **Embeddings** — `OllamaEmbedder` (e.g. `nomic-embed-text`, 768 dims) for real
  semantic search, or the zero-dependency `HashEmbedder`. Identical text is
  embedded once and cached (`hash(text) → vector`).
- **Storage** — durable JSON `FileStore` by default; **Postgres + pgvector** by
  changing one config block.

Design order was deliberately **intelligence first, storage second**: the memory
logic was built and tested against the file store, then `PgVectorStore` and
`OllamaEmbedder` dropped in behind the `store.Store` / `embed.Embedder`
interfaces — no change to `MemoryService` or the Context Builder.

```
AI CLI (cmd/ai)
   └── app            dependency wiring
       ├── workspace  loads .ai/ (canonical source of truth)
       ├── memory     embed → store → search (semantic index)
       │   ├── embed   HashEmbedder (default) | OllamaEmbedder + cache
       │   ├── chunk   paragraph-aware splitter
       │   ├── sync    incremental Obsidian/docs sync (content-hash diff)
       │   ├── ranking Reciprocal Rank Fusion (vector + keyword)
       │   └── store   FileStore (default) | MemoryStore | PgVectorStore (hybrid)
       ├── ctxbuilder builds + assembles the final prompt   ← core intelligence
       ├── agent      runtime: build context → dispatch
       └── provider   dryrun | claude | cursor | gemini | codex | agy | (any CLI)
```

## Install

```bash
# Homebrew (macOS)
brew install triforge-ai/tap/ai

# From source with the Go toolchain
go install github.com/triforge-ai/aistack/cmd/ai@latest

# Or grab a binary from the GitHub Releases page (macOS/Linux, amd64/arm64),
# or build from source (below).
```

## Quick start

> Full step-by-step setup, including Ollama and pgvector: [`SETUP.md`](SETUP.md).

```bash
make build        # or: go build -o bin/ai ./cmd/ai

mkdir demo && cd demo
../bin/ai init                                  # scaffold .ai/
../bin/ai memory sync                            # index documents/ (+ obsidian) into memory
../bin/ai memory add "use HNSW for pgvector"     # add a note (persists; or pipe via stdin)
../bin/ai memory search "vector retrieval"       # semantic search over the workspace
../bin/ai context backend "add caching"         # see the assembled prompt
../bin/ai run backend "implement an endpoint"   # run (dryrun provider by default)
```

Memory is durable across invocations (JSON store under `.ai/.cache/`); sync is
incremental (only changed files are re-embedded).

### Local embeddings with Ollama

The default `hash` embedder needs nothing but ranks lexically. For real semantic
search, point at a local Ollama model — still fully offline:

```bash
ollama serve &
ollama pull nomic-embed-text
```

```yaml
# .ai/workspace.yaml
embedder:
  type: ollama
  model: nomic-embed-text
  dimension: 768          # must match the model's output
  host: http://localhost:11434
  cache: true             # reuse vectors for identical text (default on)
```

Embeddings are cached under `.ai/.cache/embed-cache.json`, keyed by
`hash(model + text)`, so re-syncs and repeated queries don't re-run inference.

> The embedder `dimension` must match the model. When using pgvector, do not
> change an embedder's dimension without re-syncing (clear the workspace memory
> first) — mixing dimensions within a workspace breaks distance comparisons.

### Switching to pgvector

Start Postgres and point the workspace at it — nothing else changes:

```bash
make db-up          # docker compose up -d (pgvector/pgvector:pg16)
```

```yaml
# .ai/workspace.yaml
id: my-workspace
storage:
  type: pgvector
  host: localhost
  port: 5432
  user: ai
  password: ai
  db: ai_workspace
```

```bash
ai db ping          # verify the connection (also runs migrations)
ai memory sync      # now persists into Postgres
```

The schema is applied automatically on connect (idempotent), sized to the
embedder's dimension so an **HNSW** index (cosine, `m=16, ef_construction=64`)
can be built, alongside a **GIN** index for full-text keyword search. Changing
the embedder dimension against an existing table is rejected with clear guidance
(drop the table or use a fresh database).

### Hybrid search

With pgvector, `memory search` and `run` retrieval are **hybrid**: a semantic
vector query (HNSW) and a keyword/BM25 query (`ts_rank` over the GIN index) run
in parallel and are combined with **Reciprocal Rank Fusion**. This gets the best
of both — semantic queries ("how does meaning-based search work") and exact
identifiers ("ADR-014", "bug-123") both surface correctly. The file/in-memory
backends remain vector-only; the same `MemoryService.Search` call transparently
uses hybrid when the backend supports it.

> **macOS build note:** on Darwin with an older Go toolchain the internal
> linker can emit a binary missing `LC_UUID` that dyld refuses to run. `make
> build` handles this (external linker + ad-hoc codesign); upgrading Go also
> fixes it.

## Commands

| Command | Description |
| --- | --- |
| `ai init [dir]` | scaffold a `.ai/` workspace |
| `ai status` | show the loaded workspace |
| `ai run <agent> <task...>` | build context and dispatch to a provider |
| `ai chat [agent]` | interactive chat REPL with per-turn memory recall |
| `ai context <agent> <task...>` | print the assembled prompt only |
| `ai memory add <text...>` | add a note (or pipe text via stdin) |
| `ai memory search <query...>` | semantic search over workspace memory |
| `ai memory list` | list stored memories |
| `ai memory rm <id>` | delete a memory |
| `ai memory sync [name]` | incrementally sync `documents/` (+ obsidian) into memory |
| `ai db <up\|down\|status\|ping>` | manage the pgvector database via docker compose |
| `ai providers` | list agent providers and whether their CLI is installed |
| `ai health [provider...]` | check provider CLIs are installed **and runnable** (`--live` pings end-to-end) |

Flags for `run`: `--provider <name>`, `--limit <n>`.

`ai health` goes a step beyond `ai providers`: it runs each CLI's `--version`
probe to confirm it actually executes (not just that it's on `PATH`), and exits
non-zero if any checked provider is unhealthy — handy in CI or a setup script.
Check specific agents with `ai health claude codex agy`. Add `--live` to send a
tiny prompt through each agent end-to-end (this invokes the real agent and may
cost tokens). Override the probe per provider in `workspace.yaml` with
`health_args:` if a CLI lacks `--version`.

### Agent CLI providers

Providers are interchangeable runtimes. `claude` (Claude Code), `cursor`
(`cursor-agent`), `gemini`, `codex`, and `agy` are built in; the prompt is
delivered on stdin or as an argument per each CLI's convention. Pick one per
run, per agent, or per workspace:

```bash
ai providers                              # see what's installed
ai run backend "..." --provider claude    # one-off
```

```yaml
# .ai/workspace.yaml — set a default and/or register your own CLI
default_provider: claude
providers:
  - name: mycli
    bin: my-agent
    args: ["--print"]      # use "{{prompt}}" to place the prompt; else it's appended
    stdin: true            # or pipe it on stdin
```

```yaml
# .ai/agents/backend.yaml — an agent can pin its own provider
name: backend
provider: cursor
```

Resolution order is **`--provider` flag → agent definition → `default_provider` →
`dryrun`**.

### Chat

`ai chat [agent]` is an interactive REPL on top of the memory engine. Each turn
retrieves relevant memories (hybrid, when on pgvector), assembles them with the
running conversation, dispatches to the provider, and stores the exchange back
as a `chat` memory — so the session (and future sessions) can recall it.

```
ai chat backend
backend> how should I add caching here?
  …answer streams in…
backend> /memory      # show what was retrieved for the last message
backend> /save off    # stop persisting this session's turns to memory
backend> /reset       # clear the conversation
backend> /exit
```

By default every turn is stored back as a `chat` memory. To keep a session
ephemeral, toggle it at runtime with `/save off`, or disable persistence for the
workspace in `.ai/workspace.yaml`:

```yaml
chat:
  save_memory: false   # default is true
```

**Multi-provider in one session.** Switch the model mid-chat without losing the
conversation or memory — the shared transcript carries across providers, so one
model can build on another's output:

```
backend> /claude implement the storage layer
backend> /gemini optimize the above       # one-off on gemini, sees the history
backend> /use codex                        # make codex the active provider
backend> fix the bug in Save()
backend> /provider                         # show active + available providers
```

`/<provider> <msg>` runs a single message on that provider; `/<provider>` (or
`/use <provider>`) switches the active one for following turns. The default is `dryrun` (prints the assembled prompt) so real agents
never fire unless you ask — important since `cursor`/`claude` can edit files and
run shell commands.

## The `.ai/` workspace

`.ai/` is the canonical source of truth; the vector index only mirrors it.

```
.ai/
├── workspace.yaml      id and config
├── rules/*.md          always-on guidance
├── skills/*.md         reusable capabilities
├── agents/*.yaml       agent definitions (provider, rules, skills, system)
├── documents/          indexed knowledge
├── tasks/
└── memory/
```

## Roadmap

Phase 1–2 (memory logic + Obsidian sync) is done. Storage and providers next.

- [x] Memory logic: `add` / `search` / `list` / `rm`, durable `FileStore`
- [x] Incremental Obsidian/docs sync (content-hash diff)
- [x] `PgVectorStore` + docker compose + `ai db` (production semantic index)
- [x] `OllamaEmbedder` (fully-local embeddings) + embedding cache
- [x] Hybrid search (BM25 + vector, RRF) with HNSW + GIN indexes on pgvector
- [x] Pluggable agent CLI providers (claude / cursor / agy / any, via config)
- [ ] Optional cross-encoder rerank of the fused top-K
- [ ] `ai task` and git integration (`ai review` / `ai commit` / `ai pr`)
```
