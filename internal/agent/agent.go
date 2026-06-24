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
	// Write grants the provider permission to modify files / run tools (for CLI
	// agents that otherwise run read-only).
	Write bool
	// OnEvent, when set, receives the provider's typed event stream so the caller
	// can render progress (text, tool calls, ...). The runner stays presentation
	// agnostic: it forwards events without deciding how they are displayed.
	OnEvent func(provider.Event)
}

// Result is the outcome of a run.
type Result struct {
	Provider string
	Prompt   string
	Output   string
	// Streamed is true when the provider already wrote its output to the
	// terminal, so the caller should not print Output again.
	Streamed bool

	// The fields below are populated only when the provider implements
	// provider.Executor (structured backends like the stream-json CLI). For
	// plain providers they stay zero.
	Status     string                    // "completed" | "failed" | "timeout" | "aborted"
	SessionID  string                    // backend session id, for later --resume
	DurationMs int64                     // wall-clock duration of the run
	Usage      map[string]provider.Usage // token usage keyed by model
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
	if req.Write {
		if w, ok := prov.(provider.Writable); ok && w.CanWrite() {
			prov = w.WithWrite()
		}
	}

	res := Result{Provider: name, Prompt: prompt}

	// Prefer the structured Execute path so we capture token usage, the backend
	// session id, and a duration, and so progress flows through req.OnEvent.
	// Write permission is already folded in above via WithWrite, so
	// ExecOptions.Write stays false to avoid double-applying it.
	if ex, ok := prov.(provider.Executor); ok {
		rr, err := ex.Execute(ctx, prompt, provider.ExecOptions{OnEvent: req.OnEvent})
		res.Output = rr.Output
		res.Status = rr.Status
		res.SessionID = rr.SessionID
		res.DurationMs = rr.DurationMs
		res.Usage = rr.Usage
		// A supplied sink consumed (and rendered) the event stream, so the caller
		// should not reprint Output.
		res.Streamed = req.OnEvent != nil
		if err != nil {
			return res, fmt.Errorf("provider %s: %w", name, err)
		}
		return res, nil
	}

	out, err := prov.Ask(ctx, prompt)
	if err != nil {
		return res, fmt.Errorf("provider %s: %w", name, err)
	}
	res.Output = out
	return res, nil
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
