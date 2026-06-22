// Package ctxbuilder assembles the final prompt context for an agent+task. It
// is the core intelligence layer: it selects which rules, skills and memories
// the provider actually sees. If retrieval quality degrades, this is where to
// look — not the database.
package ctxbuilder

import (
	"context"

	"github.com/triforge-ai/aistack/internal/memory"
	"github.com/triforge-ai/aistack/internal/memory/service"
	"github.com/triforge-ai/aistack/internal/workspace"
)

// BuildRequest is the input to a context build.
type BuildRequest struct {
	Workspace *workspace.Workspace
	Agent     string
	Task      string
	// MemoryLimit caps how many memories are retrieved (0 → default).
	MemoryLimit int
}

// Exchange is one prior conversation turn, included when assembling a multi-turn
// (chat) prompt. Single-shot runs leave History empty.
type Exchange struct {
	Role string // display label, e.g. "User" or "Assistant (claude)"
	Text string
}

// Context is the selected material that becomes a prompt.
type Context struct {
	Agent   string
	System  string
	Rules   []string
	Skills  []string
	Memory  []memory.Memory
	History []Exchange
	Task    string
}

// Builder turns a BuildRequest into a Context.
type Builder interface {
	Build(ctx context.Context, req BuildRequest) (Context, error)
}

const defaultMemoryLimit = 5

// DefaultBuilder is the standard Context implementation.
type DefaultBuilder struct {
	memory *service.Service
}

// New returns a DefaultBuilder backed by the memory service.
func New(mem *service.Service) *DefaultBuilder {
	return &DefaultBuilder{memory: mem}
}

// Build loads the agent's rules and skills from the workspace and augments them
// with task-relevant memories retrieved from the semantic index.
func (b *DefaultBuilder) Build(ctx context.Context, req BuildRequest) (Context, error) {
	out := Context{Agent: req.Agent, Task: req.Task}

	ws := req.Workspace
	def, hasDef := ws.Agents[req.Agent]
	if hasDef {
		out.System = def.System
	}

	// Rules: those named by the agent def, else all workspace rules.
	out.Rules = selectDocs(ws.Rules, defNames(hasDef, def.Rules))
	out.Skills = selectDocs(ws.Skills, defNames(hasDef, def.Skills))

	limit := req.MemoryLimit
	if limit <= 0 {
		limit = defaultMemoryLimit
	}
	mems, err := b.memory.Search(ctx, ws.ID, req.Task, limit)
	if err != nil {
		return Context{}, err
	}
	out.Memory = mems

	return out, nil
}

// defNames returns the explicit selection if the agent def provided one,
// otherwise nil (meaning "include everything").
func defNames(hasDef bool, names []string) []string {
	if hasDef && len(names) > 0 {
		return names
	}
	return nil
}

// selectDocs returns doc contents. If want is nil, all docs are returned;
// otherwise only docs whose name is in want, in that order.
func selectDocs(docs []workspace.Doc, want []string) []string {
	if want == nil {
		out := make([]string, 0, len(docs))
		for _, d := range docs {
			out = append(out, d.Content)
		}
		return out
	}
	index := make(map[string]string, len(docs))
	for _, d := range docs {
		index[d.Name] = d.Content
	}
	var out []string
	for _, name := range want {
		if c, ok := index[name]; ok {
			out = append(out, c)
		}
	}
	return out
}
