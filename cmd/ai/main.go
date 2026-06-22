// Command ai is the entrypoint to the agent-driven AI workspace.
//
// Usage:
//
//	ai init [dir]                       scaffold a .ai/ workspace
//	ai status                           show the loaded workspace
//	ai run <agent> <task...>            run an agent against a task
//	ai context <agent> <task...>        print the assembled prompt only
//	ai chat [agent]                     interactive chat REPL with memory recall
//	ai session <list|show|rm>           manage saved chat sessions
//	ai memory add <text...>             add a note to memory (or pipe via stdin)
//	ai memory search <query...>         search workspace memory
//	ai memory list                      list stored memories
//	ai memory rm <id>                   delete a memory
//	ai memory sync [name]               sync documents/ and obsidian into memory
//	ai db <up|down|status|ping>         manage the pgvector database
//	ai providers                        list agent providers and availability
//	ai health [provider...] [--live]    check provider CLIs are installed/runnable
//	ai version                          print version
//
// Flags for run: --provider <name>, --limit <n>.
//
// The command handlers are split across sibling files by group: run.go (run,
// chat, context), memory.go, db.go, info.go (init, status, providers) and
// app.go (shared workspace/flag helpers).
package main

import (
	"fmt"
	"os"
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
	case "session", "sessions":
		return cmdSession(args)
	case "context":
		return cmdContext(args)
	case "memory":
		return cmdMemory(args)
	case "db":
		return cmdDB(args)
	case "providers":
		return cmdProviders()
	case "health", "doctor":
		return cmdHealth(args)
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
        [--provider <name>] [--session <name>] [--resume <id>] [--new]
  ai session <list|show|rm>     manage saved chat sessions
  ai context <agent> <task...>  print the assembled prompt only
  ai memory add <text...>       add a note (or pipe text via stdin)
  ai memory search <query...>   search workspace memory [--limit <n>]
  ai memory list                list stored memories
  ai memory rm <id>             delete a memory
  ai memory sync [name]         sync documents/ + obsidian into memory
  ai db <up|down|status|ping>   manage the pgvector database (docker compose)
  ai providers                  list agent providers and availability
  ai health [provider...]       check provider CLIs are installed + runnable
        [--live]                  also send a tiny prompt end-to-end
  ai version
`)
}
