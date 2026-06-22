// Command ai is the entrypoint to the agent-driven AI workspace.
//
// Usage:
//
//	ai init [dir]                       scaffold a .ai/ workspace
//	ai status                           show the loaded workspace
//	ai run <agent> <task...>            run an agent against a task
//	ai context <agent> <task...>        print the assembled prompt only
//	ai memory add <text...>             add a note to memory (or pipe via stdin)
//	ai memory search <query...>         search workspace memory
//	ai memory list                      list stored memories
//	ai memory rm <id>                   delete a memory
//	ai memory sync [name]               sync documents/ and obsidian into memory
//	ai version                          print version
//
// Flags for run: --provider <name>, --limit <n>.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ai-cli/internal/agent"
	"ai-cli/internal/app"
	"ai-cli/internal/chat"
	"ai-cli/internal/memory"
	"ai-cli/internal/memory/service"
	memsync "ai-cli/internal/memory/sync"
	"ai-cli/internal/provider"
	"ai-cli/internal/scaffold"
	"ai-cli/internal/workspace"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if err := dispatch(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func dispatch(cmd string, args []string) error {
	switch cmd {
	case "init":
		return cmdInit(args)
	case "status":
		return cmdStatus()
	case "run":
		return cmdRun(args)
	case "chat":
		return cmdChat(args)
	case "context":
		return cmdContext(args)
	case "memory":
		return cmdMemory(args)
	case "db":
		return cmdDB(args)
	case "providers":
		return cmdProviders()
	case "version", "--version", "-v":
		fmt.Println("ai", version)
		return nil
	case "help", "--help", "-h":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `ai — agent-driven AI workspace

  ai init [dir]                 scaffold a .ai/ workspace
  ai status                     show the loaded workspace
  ai run <agent> <task...>      run an agent against a task
        [--provider <name>] [--limit <n>]
  ai chat [agent]               interactive chat REPL with memory recall
        [--provider <name>]
  ai context <agent> <task...>  print the assembled prompt only
  ai memory add <text...>       add a note (or pipe text via stdin)
  ai memory search <query...>   search workspace memory [--limit <n>]
  ai memory list                list stored memories
  ai memory rm <id>             delete a memory
  ai memory sync [name]         sync documents/ + obsidian into memory
  ai db <up|down|status|ping>   manage the pgvector database (docker compose)
  ai providers                  list agent providers and availability
  ai version
`)
}

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

func cmdInit(args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	root, err := scaffold.Init(dir)
	if err != nil {
		return err
	}
	fmt.Println("created workspace at", root)
	fmt.Println("next: `ai memory sync` to index documents, then `ai run backend \"...\"`")
	return nil
}

func cmdStatus() error {
	_, ws, err := openWorkspace()
	if err != nil {
		return err
	}
	fmt.Printf("workspace: %s\n", ws.ID)
	fmt.Printf("root:      %s\n", ws.Root)
	backend := ws.Storage.Type
	if backend == "" {
		backend = "file"
	}
	fmt.Printf("storage:   %s\n", backend)
	embedder := ws.Embedder.Type
	if embedder == "" {
		embedder = "hash"
	}
	if ws.Embedder.Model != "" {
		embedder += " (" + ws.Embedder.Model + ")"
	}
	fmt.Printf("embedder:  %s\n", embedder)
	if ws.Obsidian != "" {
		fmt.Printf("obsidian:  %s\n", ws.Obsidian)
	}
	fmt.Printf("rules:     %d\n", len(ws.Rules))
	fmt.Printf("skills:    %d\n", len(ws.Skills))
	fmt.Printf("agents:    %d\n", len(ws.Agents))
	for name, def := range ws.Agents {
		provider := def.Provider
		if provider == "" {
			provider = "(default)"
		}
		fmt.Printf("  - %s [%s]\n", name, provider)
	}
	return nil
}

type runOpts struct {
	provider string
	limit    int
}

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
		default:
			positional = append(positional, args[i])
		}
	}
	return opts, positional, nil
}

func cmdRun(args []string) error {
	opts, pos, err := parseOpts(args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return fmt.Errorf("usage: ai run <agent> <task...>")
	}
	a, ws, err := openWorkspace()
	if err != nil {
		return err
	}
	res, err := a.Runner.Run(context.Background(), runRequest(ws, pos[0], strings.Join(pos[1:], " "), opts))
	if err != nil {
		return err
	}
	if res.Streamed {
		// The agent CLI already wrote its output to the terminal.
		return nil
	}
	fmt.Printf("[agent=%s provider=%s]\n\n%s\n", pos[0], res.Provider, res.Output)
	return nil
}

func cmdChat(args []string) error {
	opts, pos, err := parseOpts(args)
	if err != nil {
		return err
	}
	agentName := "backend"
	if len(pos) > 0 {
		agentName = pos[0]
	}

	a, ws, err := openWorkspace()
	if err != nil {
		return err
	}
	prov := a.Runner.ResolveProvider(ws, agentName, opts.provider)

	session := chat.New(a.Builder, a.Memory, a.Provider, ws, agentName, prov)
	return session.Run(context.Background(), os.Stdin, os.Stdout)
}

func cmdContext(args []string) error {
	opts, pos, err := parseOpts(args)
	if err != nil {
		return err
	}
	if len(pos) < 2 {
		return fmt.Errorf("usage: ai context <agent> <task...>")
	}
	opts.provider = "dryrun"
	a, ws, err := openWorkspace()
	if err != nil {
		return err
	}
	res, err := a.Runner.Run(context.Background(), runRequest(ws, pos[0], strings.Join(pos[1:], " "), opts))
	if err != nil {
		return err
	}
	fmt.Println(res.Prompt)
	return nil
}

func cmdMemory(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ai memory <add|search|list|rm|sync> ...")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return cmdMemoryAdd(rest)
	case "search":
		return cmdMemorySearch(rest)
	case "list":
		return cmdMemoryList(rest)
	case "rm":
		return cmdMemoryRm(rest)
	case "sync":
		return cmdMemorySync(rest)
	default:
		return fmt.Errorf("unknown memory subcommand %q", sub)
	}
}

func cmdMemoryAdd(args []string) error {
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		piped, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		text = strings.TrimSpace(string(piped))
	}
	if text == "" {
		return fmt.Errorf("usage: ai memory add <text...> (or pipe text via stdin)")
	}

	a, ws, err := openWorkspace()
	if err != nil {
		return err
	}
	ids, err := a.Memory.Add(context.Background(), addNote(ws.ID, text))
	if err != nil {
		return err
	}
	fmt.Printf("added %d memory chunk(s)\n", len(ids))
	for _, id := range ids {
		fmt.Println(" ", id)
	}
	return nil
}

func cmdMemorySearch(args []string) error {
	opts, pos, err := parseOpts(args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return fmt.Errorf("usage: ai memory search <query...>")
	}
	limit := opts.limit
	if limit <= 0 {
		limit = 5
	}
	a, ws, err := openWorkspace()
	if err != nil {
		return err
	}
	hits, err := a.Memory.Search(context.Background(), ws.ID, strings.Join(pos, " "), limit)
	if err != nil {
		return err
	}
	if len(hits) == 0 {
		fmt.Println("(no results — try `ai memory sync` or `ai memory add`)")
		return nil
	}
	for i, m := range hits {
		name, _ := m.Metadata["name"].(string)
		fmt.Printf("%d. [%s] %s\n   %s\n", i+1, m.Type, name, firstLine(m.Content))
	}
	return nil
}

func cmdMemoryList(_ []string) error {
	a, ws, err := openWorkspace()
	if err != nil {
		return err
	}
	mems, err := a.Memory.List(context.Background(), ws.ID)
	if err != nil {
		return err
	}
	if len(mems) == 0 {
		fmt.Println("(empty)")
		return nil
	}
	for _, m := range mems {
		name, _ := m.Metadata["name"].(string)
		fmt.Printf("%s  [%s/%s]  %s | %s\n", m.ID, m.Type, m.Source, name, firstLine(m.Content))
	}
	return nil
}

func cmdMemoryRm(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ai memory rm <id>")
	}
	a, _, err := openWorkspace()
	if err != nil {
		return err
	}
	if err := a.Memory.Delete(context.Background(), args[0]); err != nil {
		return err
	}
	fmt.Println("deleted", args[0])
	return nil
}

func cmdMemorySync(args []string) error {
	a, ws, err := openWorkspace()
	if err != nil {
		return err
	}
	sources := syncSources(ws)
	if len(args) == 1 {
		sources = filterSources(sources, args[0])
		if len(sources) == 0 {
			return fmt.Errorf("no sync source named %q", args[0])
		}
	}

	ctx := context.Background()
	for _, src := range sources {
		rep, err := a.Syncer.Sync(ctx, ws.ID, src)
		if err != nil {
			return fmt.Errorf("sync %s: %w", src.Name, err)
		}
		fmt.Printf("%-10s added=%d updated=%d removed=%d unchanged=%d\n",
			src.Name, rep.Added, rep.Updated, rep.Removed, rep.Unchanged)
	}
	return nil
}

// syncSources returns the directories to sync: always documents/, plus the
// external obsidian vault if configured.
func syncSources(ws *workspace.Workspace) []memsync.Source {
	srcs := []memsync.Source{{Name: "documents", Dir: ws.DocumentsDir()}}
	if ws.Obsidian != "" {
		srcs = append(srcs, memsync.Source{Name: "obsidian", Dir: ws.Obsidian})
	}
	return srcs
}

func filterSources(srcs []memsync.Source, name string) []memsync.Source {
	var out []memsync.Source
	for _, s := range srcs {
		if s.Name == name {
			out = append(out, s)
		}
	}
	return out
}

func cmdProviders() error {
	a, ws, err := openWorkspace()
	if err != nil {
		return err
	}
	def := ws.DefaultProvider
	if def == "" {
		def = "dryrun"
	}
	fmt.Printf("default provider: %s\n\n", def)
	fmt.Printf("%-10s %-12s %s\n", "NAME", "AVAILABLE", "")
	for _, p := range a.Provider.All() {
		avail := "—"
		if c, ok := p.(provider.Available); ok {
			if c.Available() {
				avail = "yes"
			} else {
				avail = "no (not on PATH)"
			}
		} else {
			avail = "built-in"
		}
		marker := ""
		if p.Name() == def {
			marker = "(default)"
		}
		fmt.Printf("%-10s %-12s %s\n", p.Name(), avail, marker)
	}
	return nil
}

func cmdDB(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ai db <up|down|status|ping>")
	}
	switch args[0] {
	case "up":
		return compose("up", "-d")
	case "down":
		return compose("down")
	case "status", "ps":
		return compose("ps")
	case "logs":
		return compose("logs", "--tail", "50")
	case "ping":
		return cmdDBPing()
	default:
		return fmt.Errorf("unknown db subcommand %q", args[0])
	}
}

// compose runs `docker compose -f <docker-compose.yml> <args...>` from the
// directory containing the compose file (found by walking up from cwd).
func compose(args ...string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	file, err := findUp(cwd, "docker-compose.yml")
	if err != nil {
		return fmt.Errorf("docker-compose.yml not found: %w", err)
	}
	full := append([]string{"compose", "-f", file}, args...)
	cmd := exec.Command("docker", full...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// cmdDBPing forces a real operation against the configured store; with pgvector
// this verifies connectivity and applies migrations (the store connects lazily,
// so a no-op would not detect a down database).
func cmdDBPing() error {
	a, ws, err := openWorkspace()
	if err != nil {
		return err
	}
	if _, err := a.Memory.List(context.Background(), ws.ID); err != nil {
		return err
	}
	backend := ws.Storage.Type
	if backend == "" {
		backend = "file"
	}
	fmt.Printf("ok — storage backend %q reachable\n", backend)
	return nil
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

func addNote(workspaceID, text string) service.AddInput {
	return service.AddInput{
		WorkspaceID: workspaceID,
		Type:        memory.TypeNote,
		Source:      memory.SourceCLI,
		Content:     text,
		Metadata:    map[string]any{"name": "note"},
	}
}

func runRequest(ws *workspace.Workspace, agentName, task string, opts runOpts) agent.RunRequest {
	return agent.RunRequest{
		Workspace:        ws,
		Agent:            agentName,
		Task:             task,
		ProviderOverride: opts.provider,
		MemoryLimit:      opts.limit,
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
