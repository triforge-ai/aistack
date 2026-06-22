package main

import (
	"fmt"

	"github.com/triforge-ai/aistack/internal/provider"
	"github.com/triforge-ai/aistack/internal/scaffold"
)

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
