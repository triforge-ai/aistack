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
	"github.com/triforge-ai/aistack/internal/render"
	"github.com/triforge-ai/aistack/internal/session"
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
	// toggled at runtime with /remember.
	saveMemory bool

	// writeMode grants write-capable providers permission to modify files.
	writeMode bool

	// sessions/record persist the verbatim transcript so it can be resumed.
	// Both are nil when session persistence is disabled (e.g. in tests).
	sessions session.Store
	record   *session.Record

	history  []message
	lastHits []memory.Memory
}

// New creates a chat session. saveMemory controls whether each turn is persisted
// back into memory (toggleable at runtime via /save).
func New(b ctxbuilder.Builder, mem *service.Service, reg *provider.Registry, ws *workspace.Workspace, agent, prov string, saveMemory bool) *Session {
	return &Session{builder: b, memory: mem, providers: reg, ws: ws, agent: agent, provider: prov, saveMemory: saveMemory}
}

// EnableWrite lets write-capable providers modify files for this session
// (equivalent to launching with --write).
func (s *Session) EnableWrite() { s.writeMode = true }

// Persist enables session persistence: any messages already on rec seed the
// in-memory transcript (so a resumed session continues where it left off), and
// every later turn is appended to rec and saved through store.
func (s *Session) Persist(store session.Store, rec *session.Record) {
	s.sessions = store
	s.record = rec
	for _, m := range rec.Messages {
		s.history = append(s.history, message{role: displayRole(m.Role, m.Provider), text: m.Text})
	}
}

// displayRole maps a stored session role to the transcript label used in prompts.
func displayRole(role, prov string) string {
	if role == "assistant" {
		if prov != "" {
			return "Assistant (" + prov + ")"
		}
		return "Assistant"
	}
	return "User"
}

// Run drives the REPL until EOF (Ctrl-D) or /exit.
func (s *Session) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	fmt.Fprintf(out, "chat · agent=%s · provider=%s · workspace=%s\n", s.agent, s.provider, s.ws.ID)
	if s.record != nil {
		fmt.Fprintf(out, "session: %s (id %s)", s.record.Name, s.record.ID[:8])
		if len(s.record.Messages) > 0 {
			fmt.Fprintf(out, " · resumed %d messages", len(s.record.Messages))
		}
		fmt.Fprintln(out)
	}
	s.writeStatus(out, s.provider)
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
	case "agent":
		fmt.Fprintf(out, "agent: %s (fixed for this session — relaunch `ai chat <agent>` to change)\n", s.agent)
	case "remember":
		s.toggleSave(out, rest)
	case "session":
		s.showSession(out)
	case "sessions":
		s.listSessions(ctx, out)
	case "use":
		if rest == "" {
			fmt.Fprintln(out, "usage: /use <provider>")
			break
		}
		s.switchProvider(out, rest)
	case "help":
		fmt.Fprintln(out, "commands: /<provider> [msg]  /use <provider>  /provider  /agent  /memory  /remember [on|off]  /session  /sessions  /reset  /exit")
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
		fmt.Fprintln(out, "usage: /remember [on|off]")
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
	s.writeStatus(out, name)
}

// writeStatus tells the user whether the active provider can modify files: a
// read-only warning for a write-capable provider when write mode is off, or a
// heads-up that edits are enabled when it is on.
func (s *Session) writeStatus(out io.Writer, name string) {
	p, err := s.providers.Get(name)
	if err != nil {
		return
	}
	w, ok := p.(provider.Writable)
	if !ok || !w.CanWrite() {
		return
	}
	if s.writeMode {
		fmt.Fprintf(out, "⚠ write mode ON — %s may modify files in this directory.\n", name)
	} else {
		fmt.Fprintf(out, "note: %s runs read-only — file edits won't be saved. Relaunch with --write to enable.\n", name)
	}
}

func (s *Session) showProviders(out io.Writer) {
	fmt.Fprintf(out, "active: %s\navailable: %s\n", s.provider, strings.Join(s.providers.Names(), ", "))
}

// showSession reports the active session (or that persistence is off).
func (s *Session) showSession(out io.Writer) {
	if s.record == nil {
		fmt.Fprintln(out, "(session persistence is off — start with `ai chat --session <name>`)")
		return
	}
	fmt.Fprintf(out, "session: %s  (id %s)  ·  %d messages\n", s.record.Name, s.record.ID, len(s.record.Messages))
}

// listSessions lists the workspace's saved sessions, most recent first.
func (s *Session) listSessions(ctx context.Context, out io.Writer) {
	if s.sessions == nil {
		fmt.Fprintln(out, "(session persistence is off)")
		return
	}
	recs, err := s.sessions.List(ctx, s.ws.ID)
	if err != nil {
		fmt.Fprintln(out, "error:", err)
		return
	}
	if len(recs) == 0 {
		fmt.Fprintln(out, "(no saved sessions)")
		return
	}
	for _, r := range recs {
		marker := ""
		if s.record != nil && r.ID == s.record.ID {
			marker = " *"
		}
		fmt.Fprintf(out, "  %s  %-20s %d msgs%s\n", r.ID[:8], r.Name, len(r.Messages), marker)
	}
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
	if s.writeMode {
		if w, ok := prov.(provider.Writable); ok && w.CanWrite() {
			prov = w.WithWrite()
		}
	}
	resp, streamed, err := s.dispatch(ctx, out, prov, s.assemble(built, msg))
	if err != nil {
		return err
	}
	if streamed {
		fmt.Fprintln(out) // terminate the live-rendered turn with a newline
	} else {
		fmt.Fprintln(out, resp)
	}

	// Record the speaker so a later provider sees who said what.
	s.history = append(s.history,
		message{"User", msg},
		message{"Assistant (" + providerName + ")", resp},
	)
	s.remember(ctx, msg, resp)
	s.persistTurn(ctx, out, msg, resp, providerName)
	return nil
}

// dispatch sends the prompt to the provider, preferring the structured Execute
// path so the typed event stream renders live progress to out and the backend
// session id is captured onto the record (for a later --resume). The returned
// bool reports whether the turn was streamed (rendered live), so the caller
// knows not to reprint the final text. It falls back to Ask for plain providers.
func (s *Session) dispatch(ctx context.Context, out io.Writer, prov provider.Provider, prompt string) (string, bool, error) {
	ex, ok := prov.(provider.Executor)
	if !ok {
		resp, err := prov.Ask(ctx, prompt)
		return resp, false, err
	}
	rr, err := ex.Execute(ctx, prompt, provider.ExecOptions{OnEvent: render.Terminal(out)})
	if err != nil {
		return "", true, err
	}
	if rr.SessionID != "" && s.record != nil {
		s.record.ProviderSession = rr.SessionID
	}
	return rr.Output, true, nil
}

// persistTurn appends the exchange to the active session record and saves it
// (best-effort: a save failure is surfaced but does not abort the chat).
func (s *Session) persistTurn(ctx context.Context, out io.Writer, user, assistant, providerName string) {
	if s.record == nil || s.sessions == nil {
		return
	}
	s.record.Append(session.Message{Role: "user", Text: user})
	s.record.Append(session.Message{Role: "assistant", Provider: providerName, Text: assistant})
	s.record.Provider = s.provider
	if err := s.sessions.Save(ctx, *s.record); err != nil {
		fmt.Fprintln(out, "warning: could not save session:", err)
	}
}

// maxChatMemoryReply caps how much of an assistant turn is kept as a chat
// memory. The full turn already lives verbatim in the session transcript; the
// memory is only a recall snippet, so storing the whole (possibly large) reply
// just bloats the index and feeds it back into later retrievals.
const maxChatMemoryReply = 1500

// remember stores the exchange as a chat memory (best-effort). It is a no-op
// when persistence is disabled (see /remember). The assistant reply is truncated
// to keep the semantic index from ballooning.
func (s *Session) remember(ctx context.Context, user, assistant string) {
	if !s.saveMemory || strings.TrimSpace(assistant) == "" {
		return
	}
	meta := map[string]any{"agent": s.agent}
	if s.record != nil {
		meta["session"] = s.record.ID
	}
	_, _ = s.memory.Add(ctx, service.AddInput{
		WorkspaceID: s.ws.ID,
		Type:        memory.TypeChat,
		Source:      memory.SourceAgent,
		Content:     "User: " + user + "\nAssistant: " + truncateRunes(assistant, maxChatMemoryReply),
		Metadata:    meta,
	})
}

// assemble builds the prompt using the shared context assembler, adding the
// recent conversation so chat and `ai run` produce the same layout and section
// labels (no second, divergent assembler).
func (s *Session) assemble(c ctxbuilder.Context, msg string) string {
	c.Task = msg
	c.History = s.historyExchanges()
	return ctxbuilder.Assemble(c)
}

// historyExchanges converts the recent in-memory history into assembler turns.
func (s *Session) historyExchanges() []ctxbuilder.Exchange {
	h := s.recentHistory()
	out := make([]ctxbuilder.Exchange, 0, len(h))
	for _, m := range h {
		out = append(out, ctxbuilder.Exchange{Role: m.role, Text: m.text})
	}
	return out
}

func (s *Session) recentHistory() []message {
	if len(s.history) <= historyWindow {
		return s.history
	}
	return s.history[len(s.history)-historyWindow:]
}

// truncateRunes shortens s to at most n runes, appending an ellipsis marker when
// it cuts (rune-safe so multibyte text is never split).
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…[truncated]"
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
