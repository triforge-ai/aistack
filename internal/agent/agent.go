// Package agent is the runtime that executes an agent against a task: build
// context, assemble the prompt, dispatch to the chosen provider.
package agent

import (
	"context"
	"fmt"

	"github.com/triforge-ai/aistack/internal/ctxbuilder"
	"github.com/triforge-ai/aistack/internal/provider"
	"github.com/triforge-ai/aistack/internal/workspace"
)

// Runner executes agents.
type Runner struct {
	builder   ctxbuilder.Builder
	providers *provider.Registry
	// DefaultProvider is used when neither the request nor the agent def names
	// one.
	DefaultProvider string
}

// NewRunner wires a Runner.
func NewRunner(b ctxbuilder.Builder, p *provider.Registry, defaultProvider string) *Runner {
	return &Runner{builder: b, providers: p, DefaultProvider: defaultProvider}
}

// RunRequest describes one agent invocation.
type RunRequest struct {
	Workspace *workspace.Workspace
	Agent     string
	Task      string
	// ProviderOverride forces a specific provider regardless of the agent def.
	ProviderOverride string
	MemoryLimit      int
}

// Result is the outcome of a run.
type Result struct {
	Provider string
	Prompt   string
	Output   string
	// Streamed is true when the provider already wrote its output to the
	// terminal, so the caller should not print Output again.
	Streamed bool
}

// Run builds context for the agent+task and dispatches the assembled prompt to
// the resolved provider.
func (r *Runner) Run(ctx context.Context, req RunRequest) (Result, error) {
	built, err := r.builder.Build(ctx, ctxbuilder.BuildRequest{
		Workspace:   req.Workspace,
		Agent:       req.Agent,
		Task:        req.Task,
		MemoryLimit: req.MemoryLimit,
	})
	if err != nil {
		return Result{}, fmt.Errorf("build context: %w", err)
	}
	prompt := ctxbuilder.Assemble(built)

	name := r.resolveProvider(req)
	prov, err := r.providers.Get(name)
	if err != nil {
		return Result{}, err
	}

	streamed := false
	if s, ok := prov.(provider.Streamer); ok {
		streamed = s.Streams()
	}

	out, err := prov.Ask(ctx, prompt)
	if err != nil {
		return Result{Provider: name, Prompt: prompt, Streamed: streamed}, fmt.Errorf("provider %s: %w", name, err)
	}
	return Result{Provider: name, Prompt: prompt, Output: out, Streamed: streamed}, nil
}

// ResolveProvider reports which provider a run would use, given an optional
// override and agent name. It mirrors the resolution used by Run.
func (r *Runner) ResolveProvider(ws *workspace.Workspace, agent, override string) string {
	return r.resolveProvider(RunRequest{Workspace: ws, Agent: agent, ProviderOverride: override})
}

// resolveProvider picks the provider: explicit override > agent def > default.
func (r *Runner) resolveProvider(req RunRequest) string {
	if req.ProviderOverride != "" {
		return req.ProviderOverride
	}
	if def, ok := req.Workspace.Agents[req.Agent]; ok && def.Provider != "" {
		return def.Provider
	}
	return r.DefaultProvider
}
