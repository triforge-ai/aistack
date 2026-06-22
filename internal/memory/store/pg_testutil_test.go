package store

import (
	"database/sql"
	"os"
	"testing"
)

// freshPgStore returns a PgVectorStore against AI_DATABASE_URL with a freshly
// created schema at the given dimension. It drops any existing memory table
// first so tests never inherit a dimension or data from a prior run. The test
// is skipped when no database is configured.
func freshPgStore(t *testing.T, dim int) *PgVectorStore {
	t.Helper()
	dsn := os.Getenv("AI_DATABASE_URL")
	if dsn == "" {
		t.Skip("set AI_DATABASE_URL to run pgvector tests")
	}

	raw, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := raw.Exec("DROP TABLE IF EXISTS memory"); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	raw.Close()

	s, err := OpenPgVectorStore(dsn, dim, 40)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
