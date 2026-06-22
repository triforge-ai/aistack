package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/triforge-ai/aistack/internal/session"
)

func cmdSession(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ai session <list|show|rm> ...")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list", "ls":
		return cmdSessionList()
	case "show", "cat":
		return cmdSessionShow(rest)
	case "rm", "delete":
		return cmdSessionRm(rest)
	default:
		return fmt.Errorf("unknown session subcommand %q", sub)
	}
}

// openSessions loads the workspace and returns its session store.
func openSessions() (session.Store, string, error) {
	_, ws, err := openWorkspace()
	if err != nil {
		return nil, "", err
	}
	return session.NewFileStore(ws.SessionsDir()), ws.ID, nil
}

func cmdSessionList() error {
	store, wsID, err := openSessions()
	if err != nil {
		return err
	}
	recs, err := store.List(context.Background(), wsID)
	if err != nil {
		return err
	}
	if len(recs) == 0 {
		fmt.Println("(no saved sessions — start one with `ai chat --session <name>`)")
		return nil
	}
	for _, r := range recs {
		fmt.Printf("%s  %-20s %3d msgs  [%s]  %s\n",
			r.ID[:8], r.Name, len(r.Messages), r.Agent,
			time.Unix(r.UpdatedAt, 0).Format("2006-01-02 15:04"))
	}
	return nil
}

func cmdSessionShow(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ai session show <id>")
	}
	store, wsID, err := openSessions()
	if err != nil {
		return err
	}
	id, err := resolveID(store, wsID, args[0])
	if err != nil {
		return err
	}
	r, err := store.Load(context.Background(), id)
	if err != nil {
		return err
	}
	fmt.Printf("# %s  (id %s)\nagent=%s provider=%s  ·  %d messages\n\n",
		r.Name, r.ID, r.Agent, r.Provider, len(r.Messages))
	for _, m := range r.Messages {
		who := m.Role
		if m.Role == "assistant" && m.Provider != "" {
			who = "assistant (" + m.Provider + ")"
		}
		fmt.Printf("%s:\n%s\n\n", who, m.Text)
	}
	return nil
}

func cmdSessionRm(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ai session rm <id>")
	}
	store, wsID, err := openSessions()
	if err != nil {
		return err
	}
	id, err := resolveID(store, wsID, args[0])
	if err != nil {
		return err
	}
	if err := store.Delete(context.Background(), id); err != nil {
		return err
	}
	fmt.Println("deleted", id)
	return nil
}

// resolveID resolves a user-supplied query (full id, id prefix, or exact name)
// to a single session id within the workspace.
func resolveID(store session.Store, wsID, query string) (string, error) {
	if _, err := store.Load(context.Background(), query); err == nil {
		return query, nil // exact id
	}
	recs, err := store.List(context.Background(), wsID)
	if err != nil {
		return "", err
	}
	var matches []string
	for _, r := range recs {
		if r.ID == query || r.Name == query || strings.HasPrefix(r.ID, query) {
			matches = append(matches, r.ID)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no session matching %q", query)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("%q is ambiguous (%d sessions match)", query, len(matches))
	}
}
