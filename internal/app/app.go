// Package app wires the dependency graph for the CLI from configuration.
package app

import (
	"path/filepath"

	"ai-cli/internal/agent"
	"ai-cli/internal/ctxbuilder"
	"ai-cli/internal/memory/embed"
	"ai-cli/internal/memory/service"
	"ai-cli/internal/memory/store"
	syncpkg "ai-cli/internal/memory/sync"
	"ai-cli/internal/provider"
	providercli "ai-cli/internal/provider/cli"
	"ai-cli/internal/provider/dryrun"
	"ai-cli/internal/workspace"
)

// Config controls how the app is assembled.
type Config struct {
	// DefaultProvider is used when an agent does not specify one.
	DefaultProvider string
	// CacheDir is the workspace's derived-data directory (sync state lives
	// here). When empty, an ephemeral setup is used (tests, the `init` command).
	CacheDir string
	// Storage selects the memory backend. A zero value yields an in-memory
	// store; callers that want durability set it (see StorageFromWorkspace).
	Storage store.Config
	// Embedder selects the embedding backend. A zero value yields the hash
	// embedder (see EmbedderFromWorkspace).
	Embedder embed.Config
	// Providers are CLI agent backends registered in addition to the builtins;
	// a builtin with the same name is overridden.
	Providers []providercli.Spec
}

// DefaultConfig returns the zero-dependency local configuration.
func DefaultConfig() Config {
	return Config{DefaultProvider: "dryrun"}
}

// StorageFromWorkspace maps a workspace's storage config to a store.Config,
// defaulting to a durable file store under the workspace cache dir.
func StorageFromWorkspace(ws *workspace.Workspace) store.Config {
	sc := ws.Storage
	switch store.Kind(sc.Type) {
	case store.KindPgVector:
		return store.Config{
			Kind: store.KindPgVector,
			Postgres: store.PostgresConfig{
				Host:     sc.Host,
				Port:     sc.Port,
				User:     sc.User,
				Password: sc.Password,
				DB:       sc.DB,
				SSLMode:  sc.SSLMode,
			},
		}
	case store.KindMemory:
		return store.Config{Kind: store.KindMemory}
	default:
		return store.Config{Kind: store.KindFile, Path: filepath.Join(ws.CacheDir(), "memory.json")}
	}
}

// ProvidersFromWorkspace maps a workspace's provider declarations to CLI specs.
func ProvidersFromWorkspace(ws *workspace.Workspace) []providercli.Spec {
	specs := make([]providercli.Spec, 0, len(ws.Providers))
	for _, p := range ws.Providers {
		specs = append(specs, providercli.Spec{
			Name:   p.Name,
			Bin:    p.Bin,
			Args:   p.Args,
			Stdin:  p.Stdin,
			Stream: p.Stream,
			Format: p.Format,
		})
	}
	return specs
}

// EmbedderFromWorkspace maps a workspace's embedder config to an embed.Config,
// defaulting to the hash embedder. The cache is on unless explicitly disabled
// and persists under the workspace cache dir.
func EmbedderFromWorkspace(ws *workspace.Workspace) embed.Config {
	ec := ws.Embedder
	cache := true
	if ec.Cache != nil {
		cache = *ec.Cache
	}
	cfg := embed.Config{
		Kind:      embed.Kind(ec.Type),
		Model:     ec.Model,
		Dimension: ec.Dimension,
		Host:      ec.Host,
		Cache:     cache,
		CachePath: filepath.Join(ws.CacheDir(), "embed-cache.json"),
	}
	if cfg.Kind == "" {
		cfg.Kind = embed.KindHash
	}
	return cfg
}

// App holds the wired services.
type App struct {
	Loader   *workspace.Loader
	Memory   *service.Service
	Builder  ctxbuilder.Builder
	Provider *provider.Registry
	Runner   *agent.Runner
	Syncer   *syncpkg.Syncer
}

// New assembles an App from cfg. It returns an error only if the configured
// store cannot be opened.
func New(cfg Config) (*App, error) {
	embedder, err := embed.New(cfg.Embedder)
	if err != nil {
		return nil, err
	}

	// Lock the pgvector dimension to the embedder so an HNSW index can be built
	// at the correct size, unless the workspace pins one explicitly.
	if cfg.Storage.Kind == store.KindPgVector && cfg.Storage.Postgres.Dim == 0 {
		cfg.Storage.Postgres.Dim = embedder.Dim()
	}

	st, err := store.New(cfg.Storage)
	if err != nil {
		return nil, err
	}

	mem := service.New(st, embedder)
	builder := ctxbuilder.New(mem)

	reg := provider.NewRegistry()
	reg.Register(dryrun.New())
	for _, s := range providercli.Builtins() {
		reg.Register(providercli.New(s))
	}
	for _, s := range cfg.Providers { // workspace overrides/additions
		reg.Register(providercli.New(s))
	}

	defaultProvider := cfg.DefaultProvider
	if defaultProvider == "" {
		defaultProvider = "dryrun"
	}
	runner := agent.NewRunner(builder, reg, defaultProvider)
	syncer := syncpkg.New(mem, cfg.CacheDir)

	return &App{
		Loader:   workspace.NewLoader(),
		Memory:   mem,
		Builder:  builder,
		Provider: reg,
		Runner:   runner,
		Syncer:   syncer,
	}, nil
}
