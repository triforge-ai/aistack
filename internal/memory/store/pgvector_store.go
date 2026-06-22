package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/triforge-ai/aistack/internal/memory"

	_ "github.com/lib/pq"
)

// PgVectorStore is the production Store backed by Postgres + the pgvector
// extension. It implements Store, Lister, and Hybrid (vector + BM25 keyword
// search) so MemoryService can do hybrid retrieval without learning any SQL.
// The embedding is treated as an opaque []float32 serialised to a pgvector
// literal; no ranking policy lives in this layer.
type PgVectorStore struct {
	db *sql.DB
	// dim, when > 0, fixes the embedding column dimension and enables an HNSW
	// index. When 0 the column is unbounded and search is an exact scan.
	dim      int
	efSearch int

	// Migration is deferred to the first real operation so that constructing
	// the store never opens a connection — commands that don't touch memory
	// (status, providers, db up) work even when Postgres is down.
	migrateOnce sync.Once
	migrateErr  error
}

// NewPgVectorStore wraps an existing *sql.DB (dimension unknown → exact scan).
func NewPgVectorStore(db *sql.DB) *PgVectorStore {
	return &PgVectorStore{db: db, efSearch: 40}
}

// OpenPgVectorStore prepares a store for the given DSN. It does NOT connect:
// sql.Open is lazy and migration runs on first use. dim locks the embedding
// dimension; efSearch tunes HNSW recall (0 → default 40).
func OpenPgVectorStore(dsn string, dim, efSearch int) (*PgVectorStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if efSearch <= 0 {
		efSearch = 40
	}
	return &PgVectorStore{db: db, dim: dim, efSearch: efSearch}, nil
}

// ready connects (lazily) and applies migrations exactly once. It returns a
// friendly error when Postgres is unreachable.
func (s *PgVectorStore) ready(ctx context.Context) error {
	s.migrateOnce.Do(func() {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := s.db.PingContext(pingCtx); err != nil {
			s.migrateErr = fmt.Errorf("cannot reach postgres (%w); is it running? try `ai db up`", err)
			return
		}
		s.migrateErr = s.Migrate(ctx)
	})
	return s.migrateErr
}

// Ping verifies connectivity (and applies migrations) on demand.
func (s *PgVectorStore) Ping(ctx context.Context) error { return s.ready(ctx) }

// Migrate creates the extension, table, full-text column, and indexes. It is
// idempotent. The embedding column is vector(dim) when a dimension is known
// (required for HNSW), else unbounded vector.
func (s *PgVectorStore) Migrate(ctx context.Context) error {
	// Guard against a dimension mismatch with an existing table, which would
	// otherwise surface as a cryptic insert-time error.
	if s.dim > 0 {
		if existing, ok, err := s.existingDim(ctx); err != nil {
			return err
		} else if ok && existing > 0 && existing != s.dim {
			return fmt.Errorf("memory table has embedding dimension %d but the configured embedder produces %d; "+
				"re-create the table (drop it or use a fresh database) after changing embedder dimension", existing, s.dim)
		}
	}

	embType := "vector"
	if s.dim > 0 {
		embType = fmt.Sprintf("vector(%d)", s.dim)
	}

	stmts := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS memory (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			type TEXT,
			source TEXT,
			content TEXT,
			embedding %s,
			metadata JSONB,
			created_at TIMESTAMPTZ
		)`, embType),
		// BM25/full-text support: a generated tsvector kept in sync with content.
		`ALTER TABLE memory ADD COLUMN IF NOT EXISTS content_tsv tsvector
			GENERATED ALWAYS AS (to_tsvector('english', coalesce(content, ''))) STORED`,
		`CREATE INDEX IF NOT EXISTS memory_workspace_idx ON memory (workspace_id)`,
		`CREATE INDEX IF NOT EXISTS memory_fts_idx ON memory USING GIN (content_tsv)`,
	}
	if s.dim > 0 {
		stmts = append(stmts, `CREATE INDEX IF NOT EXISTS memory_embedding_hnsw
			ON memory USING hnsw (embedding vector_cosine_ops)
			WITH (m = 16, ef_construction = 64)`)
	}
	for _, q := range stmts {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// existingDim reports the embedding column's fixed dimension if the memory
// table exists. pgvector encodes the dimension in atttypmod (-1 = unbounded).
func (s *PgVectorStore) existingDim(ctx context.Context) (dim int, ok bool, err error) {
	var typmod int
	row := s.db.QueryRowContext(ctx, `
		SELECT atttypmod FROM pg_attribute
		WHERE attrelid = to_regclass('memory') AND attname = 'embedding' AND NOT attisdropped
	`)
	switch err := row.Scan(&typmod); err {
	case sql.ErrNoRows:
		return 0, false, nil // table or column absent
	case nil:
		if typmod < 0 {
			return 0, true, nil // unbounded
		}
		return typmod, true, nil
	default:
		return 0, false, err
	}
}

// Close releases the underlying connection pool.
func (s *PgVectorStore) Close() error { return s.db.Close() }

func (s *PgVectorStore) Save(ctx context.Context, m memory.Memory) error {
	if err := s.ready(ctx); err != nil {
		return err
	}
	meta, err := json.Marshal(m.Metadata)
	if err != nil {
		return err
	}
	created := time.Unix(m.CreatedAt, 0)
	if m.CreatedAt == 0 {
		created = time.Now()
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO memory (id, workspace_id, type, source, content, embedding, metadata, created_at)
		VALUES ($1,$2,$3,$4,$5,$6::vector,$7::jsonb,$8)
		ON CONFLICT (id) DO UPDATE SET
			workspace_id = EXCLUDED.workspace_id,
			type         = EXCLUDED.type,
			source       = EXCLUDED.source,
			content      = EXCLUDED.content,
			embedding    = EXCLUDED.embedding,
			metadata     = EXCLUDED.metadata,
			created_at   = EXCLUDED.created_at
	`,
		m.ID, m.WorkspaceID, string(m.Type), string(m.Source), m.Content,
		vectorLiteral(m.Embedding), string(meta), created,
	)
	return err
}

func (s *PgVectorStore) Delete(ctx context.Context, id string) error {
	if err := s.ready(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM memory WHERE id = $1`, id)
	return err
}

// Search is the plain semantic search of the Store interface; it delegates to
// the vector path and drops the scores.
func (s *PgVectorStore) Search(ctx context.Context, q Query) ([]memory.Memory, error) {
	scored, err := s.VectorSearch(ctx, q)
	if err != nil {
		return nil, err
	}
	return memories(scored), nil
}

// VectorSearch ranks by cosine similarity using the HNSW index. The score is
// cosine similarity in [0,1] (1 - cosine distance). ef_search is applied per
// transaction so it actually affects the query regardless of pool routing.
func (s *PgVectorStore) VectorSearch(ctx context.Context, q Query) ([]Scored, error) {
	if err := s.ready(ctx); err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if s.dim > 0 {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL hnsw.ef_search = %d", s.efSearch)); err != nil {
			return nil, err
		}
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, workspace_id, type, source, content, metadata,
		       1 - (embedding <=> $2::vector) AS score
		FROM memory
		WHERE workspace_id = $1
		ORDER BY embedding <=> $2::vector
		LIMIT $3
	`, q.WorkspaceID, vectorLiteral(q.Embedding), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScored(rows)
}

// KeywordSearch ranks by BM25-style full-text relevance (ts_rank over the GIN
// index). An empty or stopword-only query yields no rows.
func (s *PgVectorStore) KeywordSearch(ctx context.Context, q Query) ([]Scored, error) {
	if err := s.ready(ctx); err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workspace_id, type, source, content, metadata,
		       ts_rank(content_tsv, plainto_tsquery('english', $2)) AS score
		FROM memory
		WHERE workspace_id = $1
		  AND content_tsv @@ plainto_tsquery('english', $2)
		ORDER BY score DESC
		LIMIT $3
	`, q.WorkspaceID, q.Text, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScored(rows)
}

// List enumerates a workspace's memories, newest first.
func (s *PgVectorStore) List(ctx context.Context, workspaceID string) ([]memory.Memory, error) {
	if err := s.ready(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workspace_id, type, source, content, metadata, 0::float8 AS score
		FROM memory
		WHERE workspace_id = $1
		ORDER BY created_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	scored, err := scanScored(rows)
	if err != nil {
		return nil, err
	}
	return memories(scored), nil
}

func scanScored(rows *sql.Rows) ([]Scored, error) {
	var out []Scored
	for rows.Next() {
		var (
			m       memory.Memory
			typ     string
			src     string
			rawMeta []byte
			score   float64
		)
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &typ, &src, &m.Content, &rawMeta, &score); err != nil {
			return nil, err
		}
		m.Type = memory.Type(typ)
		m.Source = memory.Source(src)
		if len(rawMeta) > 0 {
			_ = json.Unmarshal(rawMeta, &m.Metadata)
		}
		out = append(out, Scored{Memory: m, Score: score})
	}
	return out, rows.Err()
}

func memories(scored []Scored) []memory.Memory {
	out := make([]memory.Memory, 0, len(scored))
	for _, s := range scored {
		out = append(out, s.Memory)
	}
	return out
}

// vectorLiteral renders an embedding as a pgvector text literal: "[1,2,3]".
func vectorLiteral(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
