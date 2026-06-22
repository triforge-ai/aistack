package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/triforge-ai/aistack/internal/agent"
	"github.com/triforge-ai/aistack/internal/ctxbuilder"
	"github.com/triforge-ai/aistack/internal/memory/embed"
	"github.com/triforge-ai/aistack/internal/memory/service"
	"github.com/triforge-ai/aistack/internal/memory/store"
	"github.com/triforge-ai/aistack/internal/provider"
	"github.com/triforge-ai/aistack/internal/provider/dryrun"
	"github.com/triforge-ai/aistack/internal/workspace"
)

func TestRunnerAssemblesAndDispatches(t *testing.T) {
	mem := service.New(store.NewMemoryStore(), embed.NewHashEmbedder(64))
	builder := ctxbuilder.New(mem)

	reg := provider.NewRegistry()
	reg.Register(dryrun.New())
	runner := agent.NewRunner(builder, reg, "dryrun")

	ws := &workspace.Workspace{
		ID:    "ws",
		Rules: []workspace.Doc{{Name: "coding", Content: "be precise"}},
		Agents: map[string]workspace.AgentDef{
			"backend": {Name: "backend", System: "you are backend", Rules: []string{"coding"}},
		},
	}

	res, err := runner.Run(context.Background(), agent.RunRequest{
		Workspace: ws,
		Agent:     "backend",
		Task:      "do the thing",
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Provider != "dryrun" {
		t.Fatalf("provider = %q, want dryrun", res.Provider)
	}
	for _, want := range []string{"you are backend", "be precise", "do the thing"} {
		if !strings.Contains(res.Prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, res.Prompt)
		}
	}
}

// writableFake reports a different output depending on whether write was
// granted, so a test can detect that the write variant was used.
type writableFake struct{ wrote bool }

func (writableFake) Name() string { return "wf" }
func (w writableFake) Ask(context.Context, string) (string, error) {
	if w.wrote {
		return "WROTE", nil
	}
	return "READONLY", nil
}
func (writableFake) CanWrite() bool               { return true }
func (writableFake) WithWrite() provider.Provider { return writableFake{wrote: true} }

func TestWriteAppliesVariant(t *testing.T) {
	mem := service.New(store.NewMemoryStore(), embed.NewHashEmbedder(64))
	reg := provider.NewRegistry()
	reg.Register(writableFake{})
	runner := agent.NewRunner(ctxbuilder.New(mem), reg, "wf")
	ws := &workspace.Workspace{ID: "ws", Agents: map[string]workspace.AgentDef{"x": {Name: "x"}}}

	with, err := runner.Run(context.Background(), agent.RunRequest{Workspace: ws, Agent: "x", Task: "t", Write: true})
	if err != nil {
		t.Fatal(err)
	}
	if with.Output != "WROTE" {
		t.Fatalf("--write did not use the write variant: %q", with.Output)
	}

	without, err := runner.Run(context.Background(), agent.RunRequest{Workspace: ws, Agent: "x", Task: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if without.Output != "READONLY" {
		t.Fatalf("default run should stay read-only: %q", without.Output)
	}
}

func TestProviderOverrideWins(t *testing.T) {
	mem := service.New(store.NewMemoryStore(), embed.NewHashEmbedder(64))
	reg := provider.NewRegistry()
	reg.Register(dryrun.New())
	runner := agent.NewRunner(ctxbuilder.New(mem), reg, "claude")

	ws := &workspace.Workspace{
		ID:     "ws",
		Agents: map[string]workspace.AgentDef{"x": {Name: "x", Provider: "claude"}},
	}
	res, err := runner.Run(context.Background(), agent.RunRequest{
		Workspace: ws, Agent: "x", Task: "t", ProviderOverride: "dryrun",
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Provider != "dryrun" {
		t.Fatalf("override ignored: provider = %q", res.Provider)
	}
}
