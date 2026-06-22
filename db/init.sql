-- Enable pgvector on first container start. The CLI owns the rest of the schema
-- (table, full-text column, HNSW + GIN indexes) via an idempotent migration on
-- connect (PgVectorStore.Migrate), sized to the configured embedding dimension.
CREATE EXTENSION IF NOT EXISTS vector;
