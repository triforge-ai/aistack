// Package chat is an interactive REPL on top of the memory engine. Each turn
// retrieves relevant memories (hybrid when the store supports it), assembles a
// prompt with the running conversation, dispatches it to a provider, and stores
// the exchange back as memory so future turns and sessions can recall it.
package chat

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/triforge-ai/aistack/internal/ctxbuilder"
	"github.com/triforge-ai/aistack/internal/memory"
	"github.com/triforge-ai/aistack/internal/memory/service"
	"github.com/triforge-ai/aistack/internal/provider"
	"github.com/triforge-ai/aistack/internal/workspace"
)

// historyWindow caps how many past messages are replayed into each prompt.
const historyWindow = 20

// memoryLimit is how many memories are retrieved per turn.
const memoryLimit = 5

type message struct {
	role string // "User" | "Assistant"
	text string
}

// Session holds the state of one interactive chat.
type Session struct {
	builder   ctxbuilder.Builder
	memory    *service.Service
	providers *provider.Registry
	ws        *workspace.Workspace

	agent    string
	provider string

	// saveMemory persists each exchange back into memory when true. It can be
	// toggled at runtime with /save.
	saveMemory bool

	history  []message
	lastHits []memory.Memory
}

// New creates a chat session. saveMemory controls whether each turn is persisted
// back into memory (toggleable at runtime via /save).
func New(b ctxbuilder.Builder, mem *service.Service, reg *provider.Registry, ws *workspace.Workspace, agent, prov string, saveMemory bool) *Session {
	return &Session{builder: b, memory: mem, providers: reg, ws: ws, agent: agent, provider: prov, saveMemory: saveMemory}
}

// Run drives the REPL until EOF (Ctrl-D) or /exit.
func (s *Session) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	fmt.Fprintf(out, "chat · agent=%s · provider=%s · workspace=%s\n", s.agent, s.provider, s.ws.ID)
	fmt.Fprintln(out, "switch model with /<provider> or /use; /help for more. Ctrl-D to quit.")

	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for {
		fmt.Fprintf(out, "\n%s> ", s.agent)
		if !sc.Scan() {
			fmt.Fprintln(out)
			return sc.Err()
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			if s.command(ctx, out, line) {
				return nil
			}
			continue
		}
		if err := s.turn(ctx, out, line, s.provider); err != nil {
			fmt.Fprintln(out, "error:", err)
		}
	}
}

// command handles a slash command; it returns true when the session should end.
// Besides the system commands, any registered provider name acts as a command:
// `/<provider>` switches the active provider, and `/<provider> <msg>` runs a
// single message on that provider without changing the active one — the session
// (history + memory) carries across providers either way.
func (s *Session) command(ctx context.Context, out io.Writer, line string) bool {
	fields := strings.Fields(line)
	name := strings.TrimPrefix(fields[0], "/")
	rest := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))

	switch name {
	case "exit", "quit", "q":
		return true
	case "reset":
		s.history = nil
		fmt.Fprintln(out, "(conversation cleared)")
	case "memory":
		if len(s.lastHits) == 0 {
			fmt.Fprintln(out, "(no memories retrieved yet)")
			break
		}
		fmt.Fprintln(out, "retrieved for the last message:")
		for i, m := range s.lastHits {
			fmt.Fprintf(out, "  %d. [%s] %s\n", i+1, m.Type, firstLine(m.Content))
		}
	case "provider":
		s.showProviders(out)
	case "save":
		s.toggleSave(out, rest)
	case "use":
		if rest == "" {
			fmt.Fprintln(out, "usage: /use <provider>")
			break
		}
		s.switchProvider(out, rest)
	case "help":
		fmt.Fprintln(out, "commands: /<provider> [msg]  /use <provider>  /provider  /memory  /save [on|off]  /reset  /exit")
	default:
		// A bare provider name switches; `/<provider> <msg>` is a one-off.
		if s.providers.Has(name) {
			if rest == "" {
				s.switchProvider(out, name)
			} else if err := s.turn(ctx, out, rest, name); err != nil {
				fmt.Fprintln(out, "error:", err)
			}
			break
		}
		fmt.Fprintf(out, "unknown command %q — try /help\n", line)
	}
	return false
}

// toggleSave turns persistence of chat turns on/off. With no argument it
// reports the current state; "on"/"off" set it explicitly.
func (s *Session) toggleSave(out io.Writer, arg string) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "":
		fmt.Fprintf(out, "memory persistence is %s\n", onOff(s.saveMemory))
	case "on", "true", "yes":
		s.saveMemory = true
		fmt.Fprintln(out, "(memory persistence → on)")
	case "off", "false", "no":
		s.saveMemory = false
		fmt.Fprintln(out, "(memory persistence → off)")
	default:
		fmt.Fprintln(out, "usage: /save [on|off]")
	}
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// switchProvider sets the active provider for subsequent turns.
func (s *Session) switchProvider(out io.Writer, name string) {
	if !s.providers.Has(name) {
		fmt.Fprintf(out, "unknown provider %q — %s\n", name, s.providerList())
		return
	}
	s.provider = name
	fmt.Fprintf(out, "(active provider → %s)\n", name)
}

func (s *Session) showProviders(out io.Writer) {
	fmt.Fprintf(out, "active: %s\navailable: %s\n", s.provider, strings.Join(s.providers.Names(), ", "))
}

func (s *Session) providerList() string {
	return "available: " + strings.Join(s.providers.Names(), ", ")
}

// turn runs one user message on the given provider: retrieve → assemble →
// dispatch → store.
func (s *Session) turn(ctx context.Context, out io.Writer, msg, providerName string) error {
	built, err := s.builder.Build(ctx, ctxbuilder.BuildRequest{
		Workspace:   s.ws,
		Agent:       s.agent,
		Task:        msg,
		MemoryLimit: memoryLimit,
	})
	if err != nil {
		return err
	}
	s.lastHits = built.Memory

	prov, err := s.providers.Get(providerName)
	if err != nil {
		return err
	}
	streamed := false
	if st, ok := prov.(provider.Streamer); ok {
		streamed = st.Streams()
	}

	resp, err := prov.Ask(ctx, s.assemble(built, msg))
	if err != nil {
		return err
	}
	if !streamed {
		fmt.Fprintln(out, resp)
	}

	// Record the speaker so a later provider sees who said what.
	s.history = append(s.history,
		message{"User", msg},
		message{"Assistant (" + providerName + ")", resp},
	)
	s.remember(ctx, msg, resp)
	return nil
}

// remember stores the exchange as a chat memory (best-effort). It is a no-op
// when persistence is disabled (see /save).
func (s *Session) remember(ctx context.Context, user, assistant string) {
	if !s.saveMemory || strings.TrimSpace(assistant) == "" {
		return
	}
	_, _ = s.memory.Add(ctx, service.AddInput{
		WorkspaceID: s.ws.ID,
		Type:        memory.TypeChat,
		Source:      memory.SourceAgent,
		Content:     "User: " + user + "\nAssistant: " + assistant,
		Metadata:    map[string]any{"agent": s.agent},
	})
}

// assemble builds the prompt: system + rules + skills + retrieved memory +
// recent conversation + the new message.
func (s *Session) assemble(c ctxbuilder.Context, msg string) string {
	var b strings.Builder
	if c.System != "" {
		b.WriteString(c.System)
		b.WriteString("\n\n")
	}
	section(&b, "RULES", c.Rules)
	section(&b, "SKILLS", c.Skills)

	if len(c.Memory) > 0 {
		mems := make([]string, 0, len(c.Memory))
		for _, m := range c.Memory {
			mems = append(mems, m.Content)
		}
		section(&b, "RELEVANT MEMORY", mems)
	}

	if h := s.recentHistory(); len(h) > 0 {
		b.WriteString("[CONVERSATION SO FAR]\n")
		for _, m := range h {
			b.WriteString(m.role)
			b.WriteString(": ")
			b.WriteString(m.text)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("[USER]\n")
	b.WriteString(msg)
	b.WriteString("\n")
	return b.String()
}

func (s *Session) recentHistory() []message {
	if len(s.history) <= historyWindow {
		return s.history
	}
	return s.history[len(s.history)-historyWindow:]
}

func section(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	b.WriteString("[" + title + "]\n")
	b.WriteString(strings.Join(items, "\n\n"))
	b.WriteString("\n\n")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
