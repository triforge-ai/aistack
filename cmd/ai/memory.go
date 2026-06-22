package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/triforge-ai/aistack/internal/memory"
	"github.com/triforge-ai/aistack/internal/memory/service"
	memsync "github.com/triforge-ai/aistack/internal/memory/sync"
	"github.com/triforge-ai/aistack/internal/workspace"
)

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

func addNote(workspaceID, text string) service.AddInput {
	return service.AddInput{
		WorkspaceID: workspaceID,
		Type:        memory.TypeNote,
		Source:      memory.SourceCLI,
		Content:     text,
		Metadata:    map[string]any{"name": "note"},
	}
}
