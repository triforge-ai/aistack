package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/triforge-ai/aistack/internal/app"
	"github.com/triforge-ai/aistack/internal/workspace"
)

// openWorkspace finds the .ai/ workspace from the cwd upward, builds the app
// with a durable store under .ai/.cache, and loads the workspace.
func openWorkspace() (*app.App, *workspace.Workspace, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	root, err := workspace.Find(cwd)
	if err != nil {
		return nil, nil, err
	}

	// Load the workspace first: it declares which storage backend to use.
	ws, err := workspace.NewLoader().Load(root)
	if err != nil {
		return nil, nil, err
	}

	cfg := app.DefaultConfig()
	cfg.CacheDir = ws.CacheDir()
	cfg.Storage = app.StorageFromWorkspace(ws)
	cfg.Embedder = app.EmbedderFromWorkspace(ws)
	cfg.Providers = app.ProvidersFromWorkspace(ws)
	cfg.DefaultProvider = ws.DefaultProvider

	a, err := app.New(cfg)
	if err != nil {
		return nil, nil, err
	}
	return a, ws, nil
}

type runOpts struct {
	provider string
	limit    int
	write    bool
}

// parseOpts splits args into the shared --provider/--limit flags and the
// remaining positional arguments.
func parseOpts(args []string) (runOpts, []string, error) {
	var opts runOpts
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--provider":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("--provider needs a value")
			}
			i++
			opts.provider = args[i]
		case "--limit":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("--limit needs a value")
			}
			i++
			if _, err := fmt.Sscanf(args[i], "%d", &opts.limit); err != nil {
				return opts, nil, fmt.Errorf("--limit: %w", err)
			}
		case "--write", "--yolo":
			opts.write = true
		default:
			positional = append(positional, args[i])
		}
	}
	return opts, positional, nil
}

// findUp returns the path to name, searching dir and its ancestors.
func findUp(dir, name string) (string, error) {
	cur := dir
	for {
		candidate := cur + string(os.PathSeparator) + name
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("not found from %s upward", dir)
		}
		cur = parent
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
