package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the parsed .ai/workspace.yaml.
type Config struct {
	ID              string           `yaml:"id"`
	Obsidian        string           `yaml:"obsidian"`
	Storage         StorageConfig    `yaml:"storage"`
	Embedder        EmbedderConfig   `yaml:"embedder"`
	Chat            ChatConfig       `yaml:"chat"`
	Providers       []ProviderConfig `yaml:"providers"`
	DefaultProvider string           `yaml:"default_provider"`
}

// ChatConfig tunes the interactive REPL. SaveMemory controls whether each turn
// is persisted back into memory; nil means "default on" (set `save_memory:
// false` to keep chat ephemeral and avoid memory bloat / feedback loops).
type ChatConfig struct {
	SaveMemory *bool `yaml:"save_memory"`
}

// ProviderConfig declares an agent CLI backend. It adds to (or overrides by
// name) the built-in claude/cursor/agy providers.
type ProviderConfig struct {
	Name   string   `yaml:"name"`
	Bin    string   `yaml:"bin"`
	Args   []string `yaml:"args"`
	Stdin  bool     `yaml:"stdin"`
	Stream bool     `yaml:"stream"`
	// Format names the output encoding, e.g. "stream-json" for Claude Code's
	// event stream (rendered as live progress). Empty means plain text.
	Format string `yaml:"format"`
	// HealthArgs overrides the liveness probe used by `ai health` (default
	// `--version`); useful for a CLI that does not support --version.
	HealthArgs []string `yaml:"health_args"`
}

// EmbedderConfig selects the embedding backend. Type defaults to "hash" (no
// dependencies); set it to "ollama" for fully-local model embeddings.
type EmbedderConfig struct {
	Type      string `yaml:"type"`
	Model     string `yaml:"model"`
	Dimension int    `yaml:"dimension"`
	Host      string `yaml:"host"`
	// Cache enables the embedding cache; nil means "default on".
	Cache *bool `yaml:"cache"`
}

// StorageConfig selects the memory backend. Type defaults to "file"; set it to
// "pgvector" to use Postgres.
type StorageConfig struct {
	Type     string `yaml:"type"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DB       string `yaml:"db"`
	SSLMode  string `yaml:"sslmode"`
}

// Loader reads workspaces from disk.
type Loader struct{}

// NewLoader returns a Loader.
func NewLoader() *Loader { return &Loader{} }

// Find walks up from dir looking for a .ai/ directory and returns its path.
func Find(dir string) (string, error) {
	cur, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(cur, ".ai")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("no .ai/ workspace found from %s upward", dir)
		}
		cur = parent
	}
}

// Load reads the workspace rooted at the given .ai/ directory.
func (l *Loader) Load(root string) (*Workspace, error) {
	ws := &Workspace{Root: root, Agents: map[string]AgentDef{}}

	cfg, err := readConfig(filepath.Join(root, "workspace.yaml"))
	if err != nil {
		return nil, err
	}
	ws.ID = cfg.ID
	if ws.ID == "" {
		ws.ID = filepath.Base(filepath.Dir(root))
	}
	if cfg.Obsidian != "" {
		ws.Obsidian = os.ExpandEnv(cfg.Obsidian)
	}
	ws.Storage = cfg.Storage
	ws.Embedder = cfg.Embedder
	ws.Chat = cfg.Chat
	ws.Providers = cfg.Providers
	ws.DefaultProvider = cfg.DefaultProvider

	if ws.Rules, err = loadDocs(filepath.Join(root, "rules")); err != nil {
		return nil, err
	}
	if ws.Skills, err = loadDocs(filepath.Join(root, "skills")); err != nil {
		return nil, err
	}
	if err := loadAgents(filepath.Join(root, "agents"), ws.Agents); err != nil {
		return nil, err
	}
	return ws, nil
}

func readConfig(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	return cfg, yaml.Unmarshal(data, &cfg)
}

// loadDocs reads every *.md file in dir (non-recursively). A missing dir is not
// an error — it just yields no docs.
func loadDocs(dir string) ([]Doc, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var docs []Doc
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		docs = append(docs, Doc{
			Name:    strings.TrimSuffix(e.Name(), ".md"),
			Path:    path,
			Content: string(data),
		})
	}
	return docs, nil
}

func loadAgents(dir string, into map[string]AgentDef) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !(strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		var def AgentDef
		if err := yaml.Unmarshal(data, &def); err != nil {
			return fmt.Errorf("parse agent %s: %w", name, err)
		}
		if def.Name == "" {
			def.Name = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
		}
		into[def.Name] = def
	}
	return nil
}
