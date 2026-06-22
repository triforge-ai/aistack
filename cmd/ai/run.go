package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/triforge-ai/aistack/internal/agent"
	"github.com/triforge-ai/aistack/internal/chat"
	"github.com/triforge-ai/aistack/internal/session"
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
	var providerOverride, sessionName, resumeID string
	forceNew := false
	var pos []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--provider":
			if i+1 >= len(args) {
				return fmt.Errorf("--provider needs a value")
			}
			i++
			providerOverride = args[i]
		case "--session":
			if i+1 >= len(args) {
				return fmt.Errorf("--session needs a value")
			}
			i++
			sessionName = args[i]
		case "--resume":
			if i+1 >= len(args) {
				return fmt.Errorf("--resume needs a value")
			}
			i++
			resumeID = args[i]
		case "--new":
			forceNew = true
		default:
			pos = append(pos, args[i])
		}
	}
	agentName := "backend"
	if len(pos) > 0 {
		agentName = pos[0]
	}

	a, ws, err := openWorkspace()
	if err != nil {
		return err
	}
	prov := a.Runner.ResolveProvider(ws, agentName, providerOverride)

	store := session.NewFileStore(ws.SessionsDir())
	rec, err := resolveSession(store, ws, agentName, prov, sessionName, resumeID, forceNew)
	if err != nil {
		return err
	}

	sess := chat.New(a.Builder, a.Memory, a.Provider, ws, agentName, prov, ws.SaveChatMemory())
	sess.Persist(store, rec)
	return sess.Run(context.Background(), os.Stdin, os.Stdout)
}

// resolveSession picks the session to persist into: --resume loads by id;
// --session opens an existing session with that name (unless --new), else a
// fresh one is created.
func resolveSession(store session.Store, ws *workspace.Workspace, agent, prov, name, resumeID string, forceNew bool) (*session.Record, error) {
	ctx := context.Background()
	if resumeID != "" {
		id, err := resolveID(store, ws.ID, resumeID)
		if err != nil {
			return nil, err
		}
		rec, err := store.Load(ctx, id)
		if err != nil {
			return nil, err
		}
		return &rec, nil
	}
	if name != "" && !forceNew {
		recs, err := store.List(ctx, ws.ID)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			if r.Name == name {
				found := r
				return &found, nil
			}
		}
	}
	rec := session.New(name, ws.ID, agent, prov)
	return &rec, nil
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
