package recipe

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/template"
)

// Executor performs the side-effecting work of a step. Keeping it an interface
// lets the recipe package stay free of provider/agent/shell dependencies (and
// makes the runner unit-testable with a fake).
type Executor interface {
	// RunAgent runs an agent (with an optional provider override) on a prompt
	// and returns its text output.
	RunAgent(ctx context.Context, agent, provider, prompt string) (string, error)
	// RunShell runs a shell command and returns its captured stdout.
	RunShell(ctx context.Context, cmd string) (string, error)
}

// Runner executes recipes step by step.
type Runner struct {
	exec       Executor
	allowShell bool
}

// NewRunner builds a Runner. Shell steps only execute when allowShell is true;
// otherwise they fail fast (recipes can run arbitrary commands, so this is
// opt-in).
func NewRunner(exec Executor, allowShell bool) *Runner {
	return &Runner{exec: exec, allowShell: allowShell}
}

// Result is the outcome of a recipe run: each step's output, in order.
type Result struct {
	Outputs map[string]string
	Order   []string
}

// Run executes rec's steps sequentially. Each step's cmd/prompt is templated
// with the outputs collected so far and the supplied vars; the output is stored
// under the step id for later steps. Progress is written to out.
func (r *Runner) Run(ctx context.Context, rec Recipe, vars map[string]string, out io.Writer) (Result, error) {
	res := Result{Outputs: map[string]string{}}

	for _, s := range rec.Steps {
		fmt.Fprintf(out, "→ %s (%s)\n", s.ID, s.kind())

		switch s.kind() {
		case KindShell:
			if !r.allowShell {
				return res, fmt.Errorf("step %q is a shell step; re-run with --allow-shell to permit it", s.ID)
			}
			cmd, err := render(s.Cmd, res.Outputs, vars)
			if err != nil {
				return res, fmt.Errorf("step %q: %w", s.ID, err)
			}
			output, err := r.exec.RunShell(ctx, cmd)
			if err != nil {
				return res, fmt.Errorf("step %q (shell): %w", s.ID, err)
			}
			r.record(&res, s.ID, output)

		case KindAgent:
			prompt, err := render(s.Prompt, res.Outputs, vars)
			if err != nil {
				return res, fmt.Errorf("step %q: %w", s.ID, err)
			}
			output, err := r.exec.RunAgent(ctx, s.Agent, s.Provider, prompt)
			if err != nil {
				return res, fmt.Errorf("step %q (agent): %w", s.ID, err)
			}
			r.record(&res, s.ID, output)
		}
	}
	return res, nil
}

func (r *Runner) record(res *Result, id, output string) {
	res.Outputs[id] = output
	res.Order = append(res.Order, id)
}

// render expands a template string. Two functions are available:
//
//	{{ step "id" }}   the output of an earlier step
//	{{ var "name" }}  a value passed via --var name=value
func render(tmpl string, outputs, vars map[string]string) (string, error) {
	if !strings.Contains(tmpl, "{{") {
		return tmpl, nil // fast path: no templating
	}
	funcs := template.FuncMap{
		"step": func(id string) (string, error) {
			v, ok := outputs[id]
			if !ok {
				return "", fmt.Errorf("no output from step %q (is it defined earlier?)", id)
			}
			return v, nil
		},
		"var": func(name string) string { return vars[name] },
	}
	t, err := template.New("recipe").Funcs(funcs).Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("template: %w", err)
	}
	var b strings.Builder
	if err := t.Execute(&b, nil); err != nil {
		return "", fmt.Errorf("template: %w", err)
	}
	return b.String(), nil
}
