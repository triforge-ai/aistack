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
