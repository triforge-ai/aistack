package store

import "fmt"

// Kind selects a Store backend.
type Kind string

const (
	KindMemory   Kind = "memory"   // ephemeral, in-process (tests)
	KindFile     Kind = "file"     // durable JSON file (default dev backend)
	KindPgVector Kind = "pgvector" // production semantic index
)

// PostgresConfig holds connection settings for the pgvector backend.
type PostgresConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DB       string
	SSLMode  string
	// Dim locks the embedding dimension so an HNSW index can be built. When 0,
	// the column stays unbounded and search falls back to an exact scan.
	Dim int
	// EfSearch tunes HNSW recall at query time (default 40).
	EfSearch int
}

// DSN renders a lib/pq connection string.
func (c PostgresConfig) DSN() string {
	host := c.Host
	if host == "" {
		host = "localhost"
	}
	port := c.Port
	if port == 0 {
		port = 5432
	}
	ssl := c.SSLMode
	if ssl == "" {
		ssl = "disable"
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		host, port, c.User, c.Password, c.DB, ssl)
}

// Config selects and parameterises a Store backend. It is the single seam the
// factory reads; adding a backend means adding a Kind, not changing callers.
type Config struct {
	Kind Kind
	// Path is the JSON file location for KindFile.
	Path string
	// Postgres configures KindPgVector. If DSN is non-empty it takes precedence.
	Postgres PostgresConfig
	DSN      string
}

// New builds a Store from cfg. It is the factory the rest of the system uses so
// MemoryService and the Context Builder never learn which backend is active.
func New(cfg Config) (Store, error) {
	switch cfg.Kind {
	case KindMemory, "":
		return NewMemoryStore(), nil
	case KindFile:
		return NewFileStore(cfg.Path)
	case KindPgVector:
		dsn := cfg.DSN
		if dsn == "" {
			dsn = cfg.Postgres.DSN()
		}
		return OpenPgVectorStore(dsn, cfg.Postgres.Dim, cfg.Postgres.EfSearch)
	default:
		return nil, fmt.Errorf("unknown store kind %q", cfg.Kind)
	}
}
