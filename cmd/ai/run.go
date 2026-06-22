package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/triforge-ai/aistack/internal/agent"
	"github.com/triforge-ai/aistack/internal/chat"
	"github.com/triforge-ai/aistack/internal/workspace"
)

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

	session := chat.New(a.Builder, a.Memory, a.Provider, ws, agentName, prov, ws.SaveChatMemory())
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

func runRequest(ws *workspace.Workspace, agentName, task string, opts runOpts) agent.RunRequest {
	return agent.RunRequest{
		Workspace:        ws,
		Agent:            agentName,
		Task:             task,
		ProviderOverride: opts.provider,
		MemoryLimit:      opts.limit,
	}
}
