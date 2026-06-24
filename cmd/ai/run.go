package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/triforge-ai/aistack/internal/agent"
	"github.com/triforge-ai/aistack/internal/app"
	"github.com/triforge-ai/aistack/internal/chat"
	"github.com/triforge-ai/aistack/internal/provider"
	"github.com/triforge-ai/aistack/internal/render"
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
	writeHint(a, ws, pos[0], opts.provider, opts.write)
	req := runRequest(ws, pos[0], strings.Join(pos[1:], " "), opts)
	req.OnEvent = render.Terminal(os.Stdout)
	res, err := a.Runner.Run(context.Background(), req)
	if err != nil {
		return err
	}
	if res.Streamed {
		fmt.Fprintln(os.Stdout) // terminate the live-rendered output with a newline
	} else {
		// Plain providers don't render their own output — print it.
		fmt.Printf("[agent=%s provider=%s]\n\n%s\n", pos[0], res.Provider, res.Output)
	}
	if footer := runFooter(pos[0], res); footer != "" {
		fmt.Fprintln(os.Stderr, footer)
	}
	return nil
}

// runFooter builds a compact one-line summary of a run — duration and token
// usage — for backends that report them. It returns "" when there is nothing
// structured to show (e.g. a plain provider), so simple runs stay quiet.
func runFooter(agentName string, res agent.Result) string {
	var parts []string
	if res.DurationMs > 0 {
		parts = append(parts, fmt.Sprintf("%.1fs", float64(res.DurationMs)/1000))
	}
	if u := totalUsage(res.Usage); u != (provider.Usage{}) {
		seg := fmt.Sprintf("%s in / %s out", humanTokens(u.InputTokens), humanTokens(u.OutputTokens))
		if cache := u.CacheReadTokens + u.CacheWriteTokens; cache > 0 {
			seg += fmt.Sprintf(" (+%s cache)", humanTokens(cache))
		}
		parts = append(parts, seg)
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("[agent=%s provider=%s · %s]", agentName, res.Provider, strings.Join(parts, " · "))
}

// totalUsage sums per-model usage into a single tally for the footer.
func totalUsage(usage map[string]provider.Usage) provider.Usage {
	var t provider.Usage
	for _, u := range usage {
		t.InputTokens += u.InputTokens
		t.OutputTokens += u.OutputTokens
		t.CacheReadTokens += u.CacheReadTokens
		t.CacheWriteTokens += u.CacheWriteTokens
	}
	return t
}

// humanTokens renders a token count compactly (e.g. 1.2k, 340).
func humanTokens(n int64) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func cmdChat(args []string) error {
	var providerOverride, sessionName, resumeID string
	forceNew := false
	writeMode := false
	var pos []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--provider":
			if i+1 >= len(args) {
				return fmt.Errorf("--provider needs a value")
			}
			i++
			providerOverride = args[i]
		case "--write", "--yolo":
			writeMode = true
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
	if writeMode {
		sess.EnableWrite()
	}
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
		Write:            opts.write,
	}
}

// writeHint warns, on a write-capable provider that is running read-only, that
// file edits will be silently discarded unless --write is passed. It returns the
// resolved provider name for reuse.
func writeHint(a *app.App, ws *workspace.Workspace, agentName, override string, write bool) {
	if write {
		return
	}
	name := a.Runner.ResolveProvider(ws, agentName, override)
	if p, err := a.Provider.Get(name); err == nil {
		if w, ok := p.(provider.Writable); ok && w.CanWrite() {
			fmt.Fprintf(os.Stderr,
				"note: %s runs read-only — file edits won't be saved. Re-run with --write to let it modify files.\n", name)
		}
	}
}
