package chat_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/triforge-ai/aistack/internal/chat"
	"github.com/triforge-ai/aistack/internal/ctxbuilder"
	"github.com/triforge-ai/aistack/internal/memory"
	"github.com/triforge-ai/aistack/internal/memory/embed"
	"github.com/triforge-ai/aistack/internal/memory/service"
	"github.com/triforge-ai/aistack/internal/memory/store"
	"github.com/triforge-ai/aistack/internal/provider"
	"github.com/triforge-ai/aistack/internal/provider/dryrun"
	"github.com/triforge-ai/aistack/internal/session"
	"github.com/triforge-ai/aistack/internal/workspace"
)

func newSession(t *testing.T) (*chat.Session, *service.Service) {
	t.Helper()
	mem := service.New(store.NewMemoryStore(), embed.NewHashEmbedder(64))
	reg := provider.NewRegistry()
	reg.Register(dryrun.New())
	ws := &workspace.Workspace{
		ID:    "ws",
		Rules: []workspace.Doc{{Name: "coding", Content: "be precise"}},
		Agents: map[string]workspace.AgentDef{
			"backend": {Name: "backend", System: "you are backend"},
		},
	}
	s := chat.New(ctxbuilder.New(mem), mem, reg, ws, "backend", "dryrun", true)
	return s, mem
}

func TestChatRetrievesMemoryAndAssembles(t *testing.T) {
	ctx := context.Background()
	s, mem := newSession(t)
	if _, err := mem.Add(ctx, service.AddInput{
		WorkspaceID: "ws", Type: memory.TypeNote, Content: "caching uses an LRU in front of storage",
	}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	in := strings.NewReader("how should I add caching?\n/exit\n")
	if err := s.Run(ctx, in, &out); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	for _, want := range []string{"you are backend", "be precise", "caching uses an LRU", "[USER]", "how should I add caching?"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestChatStoresConversationAsMemory(t *testing.T) {
	ctx := context.Background()
	s, mem := newSession(t)

	in := strings.NewReader("remember this\n/exit\n")
	if err := s.Run(ctx, in, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	hits, err := mem.List(ctx, "ws")
	if err != nil {
		t.Fatal(err)
	}
	var chatMems int
	for _, m := range hits {
		if m.Type == memory.TypeChat {
			chatMems++
		}
	}
	if chatMems == 0 {
		t.Fatal("expected the exchange to be stored as a chat memory")
	}
}

func TestChatSaveOffDoesNotPersist(t *testing.T) {
	ctx := context.Background()
	s, mem := newSession(t)

	// /remember off must stop the exchange from being written to memory.
	in := strings.NewReader("/remember off\nremember this\n/exit\n")
	if err := s.Run(ctx, in, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	hits, err := mem.List(ctx, "ws")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range hits {
		if m.Type == memory.TypeChat {
			t.Fatalf("expected no chat memory after /save off, found: %q", m.Content)
		}
	}
}

func TestChatPersistsAndResumesSession(t *testing.T) {
	ctx := context.Background()
	store := session.NewFileStore(t.TempDir())

	// First session: one turn, persisted.
	s1, _ := newSession(t)
	rec := session.New("work", "ws", "backend", "dryrun")
	s1.Persist(store, &rec)
	if err := s1.Run(ctx, strings.NewReader("first message\n/exit\n"), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	saved, err := store.Load(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Messages) != 2 || saved.Messages[0].Text != "first message" {
		t.Fatalf("turn not persisted to session: %+v", saved.Messages)
	}

	// Resume: a new Session seeded from the saved record must replay the prior
	// turn into the assembled prompt.
	s2, _ := newSession(t)
	s2.Persist(store, &saved)
	var out bytes.Buffer
	if err := s2.Run(ctx, strings.NewReader("second message\n/exit\n"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "first message") {
		t.Fatalf("resumed session did not replay prior history:\n%s", out.String())
	}

	reloaded, _ := store.Load(ctx, rec.ID)
	if len(reloaded.Messages) != 4 {
		t.Fatalf("resumed turn not appended: want 4 messages, got %d", len(reloaded.Messages))
	}
}

func TestChatExitImmediately(t *testing.T) {
	ctx := context.Background()
	s, _ := newSession(t)
	if err := s.Run(ctx, strings.NewReader("/exit\n"), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}

// altProvider is a stand-in second provider for switching tests.
type altProvider struct{}

func (altProvider) Name() string                                { return "alt" }
func (altProvider) Ask(context.Context, string) (string, error) { return "ALT-REPLY", nil }

func newMultiSession(t *testing.T) *chat.Session {
	t.Helper()
	mem := service.New(store.NewMemoryStore(), embed.NewHashEmbedder(64))
	reg := provider.NewRegistry()
	reg.Register(dryrun.New())
	reg.Register(altProvider{})
	ws := &workspace.Workspace{ID: "ws", Agents: map[string]workspace.AgentDef{"backend": {Name: "backend"}}}
	return chat.New(ctxbuilder.New(mem), mem, reg, ws, "backend", "dryrun", true)
}

func TestChatSwitchProvider(t *testing.T) {
	var out bytes.Buffer
	in := strings.NewReader("/use alt\n/provider\n/use bogus\n/exit\n")
	if err := newMultiSession(t).Run(context.Background(), in, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "active provider → alt") {
		t.Fatalf("/use did not switch:\n%s", got)
	}
	if !strings.Contains(got, "active: alt") {
		t.Fatalf("/provider did not report active:\n%s", got)
	}
	if !strings.Contains(got, `unknown provider "bogus"`) {
		t.Fatalf("/use bogus should error:\n%s", got)
	}
}

func TestChatOneShotProviderDoesNotSwitch(t *testing.T) {
	var out bytes.Buffer
	// One-shot on alt, then a normal message which must still go to the default.
	in := strings.NewReader("/alt one shot please\n/provider\n/exit\n")
	if err := newMultiSession(t).Run(context.Background(), in, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "ALT-REPLY") {
		t.Fatalf("one-shot did not run on alt:\n%s", got)
	}
	if !strings.Contains(got, "active: dryrun") {
		t.Fatalf("one-shot must not change the active provider:\n%s", got)
	}
}
