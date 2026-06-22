package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/triforge-ai/aistack/internal/agent"
	"github.com/triforge-ai/aistack/internal/recipe"
	"github.com/triforge-ai/aistack/internal/workspace"
)

func cmdRecipe(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ai recipe <list|show|run> ...")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list", "ls":
		return cmdRecipeList()
	case "show", "cat":
		return cmdRecipeShow(rest)
	case "run":
		return cmdRecipeRun(rest)
	default:
		return fmt.Errorf("unknown recipe subcommand %q", sub)
	}
}

// recipePath resolves a recipe name (with or without extension) to a file.
func recipePath(ws *workspace.Workspace, name string) (string, error) {
	name = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
	for _, ext := range []string{".yaml", ".yml"} {
		p := filepath.Join(ws.RecipesDir(), name+ext)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no recipe named %q in %s", name, ws.RecipesDir())
}

func cmdRecipeList() error {
	_, ws, err := openWorkspace()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(ws.RecipesDir())
	if os.IsNotExist(err) || len(entries) == 0 {
		fmt.Println("(no recipes — add one under .ai/recipes/<name>.yaml)")
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !(strings.HasSuffix(n, ".yaml") || strings.HasSuffix(n, ".yml")) {
			continue
		}
		stem := strings.TrimSuffix(strings.TrimSuffix(n, ".yaml"), ".yml")
		line := stem
		if r, err := recipe.Load(filepath.Join(ws.RecipesDir(), n)); err == nil {
			line = fmt.Sprintf("%-20s %d steps", stem, len(r.Steps))
		}
		fmt.Println(line)
	}
	return nil
}

func cmdRecipeShow(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ai recipe show <name>")
	}
	_, ws, err := openWorkspace()
	if err != nil {
		return err
	}
	p, err := recipePath(ws, args[0])
	if err != nil {
		return err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	fmt.Print(string(data))
	return nil
}

func cmdRecipeRun(args []string) error {
	allowShell := false
	vars := map[string]string{}
	var pos []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--allow-shell":
			allowShell = true
		case "--var":
			if i+1 >= len(args) {
				return fmt.Errorf("--var needs key=value")
			}
			i++
			k, v, ok := strings.Cut(args[i], "=")
			if !ok {
				return fmt.Errorf("--var must be key=value, got %q", args[i])
			}
			vars[k] = v
		default:
			pos = append(pos, args[i])
		}
	}
	if len(pos) != 1 {
		return fmt.Errorf("usage: ai recipe run <name> [--var k=v] [--allow-shell]")
	}

	a, ws, err := openWorkspace()
	if err != nil {
		return err
	}
	p, err := recipePath(ws, pos[0])
	if err != nil {
		return err
	}
	rec, err := recipe.Load(p)
	if err != nil {
		return err
	}

	runner := recipe.NewRunner(cliExecutor{runner: a.Runner, ws: ws}, allowShell)
	res, err := runner.Run(context.Background(), rec, vars, os.Stdout)
	if err != nil {
		return err
	}

	fmt.Println("\n--- outputs ---")
	for _, id := range res.Order {
		fmt.Printf("\n[%s]\n%s\n", id, res.Outputs[id])
	}
	return nil
}

// cliExecutor bridges the recipe engine to the real agent runtime and shell.
type cliExecutor struct {
	runner *agent.Runner
	ws     *workspace.Workspace
}

func (e cliExecutor) RunAgent(ctx context.Context, agentName, provider, prompt string) (string, error) {
	res, err := e.runner.Run(ctx, agent.RunRequest{
		Workspace:        e.ws,
		Agent:            agentName,
		Task:             prompt,
		ProviderOverride: provider,
	})
	if err != nil {
		return "", err
	}
	return res.Output, nil
}

func (e cliExecutor) RunShell(ctx context.Context, cmd string) (string, error) {
	var buf bytes.Buffer
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	c.Stdout = &buf
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}
