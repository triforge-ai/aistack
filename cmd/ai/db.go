package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

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
