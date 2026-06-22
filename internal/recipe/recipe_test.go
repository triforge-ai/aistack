package recipe_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/triforge-ai/aistack/internal/recipe"
)

// fakeExec records calls and returns canned/echoed output.
type fakeExec struct {
	shellCmds   []string
	agentCalls  []string // "agent|provider|prompt"
	shellOutput func(cmd string) string
	agentOutput func(prompt string) string
}

func (f *fakeExec) RunShell(_ context.Context, cmd string) (string, error) {
	f.shellCmds = append(f.shellCmds, cmd)
	if f.shellOutput != nil {
		return f.shellOutput(cmd), nil
	}
	return "SHELL:" + cmd, nil
}

func (f *fakeExec) RunAgent(_ context.Context, agent, provider, prompt string) (string, error) {
	f.agentCalls = append(f.agentCalls, fmt.Sprintf("%s|%s|%s", agent, provider, prompt))
	if f.agentOutput != nil {
		return f.agentOutput(prompt), nil
	}
	return "AGENT:" + prompt, nil
}

const pipelineYAML = `
name: pipeline
steps:
  - id: changes
    run: shell
    cmd: git diff --stat
  - id: summary
    run: agent
    agent: backend
    provider: gemini
    prompt: "Summarize:\n{{ step \"changes\" }}"
`

func TestParseValid(t *testing.T) {
	r, err := recipe.Parse([]byte(pipelineYAML))
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "pipeline" || len(r.Steps) != 2 {
		t.Fatalf("unexpected recipe: %+v", r)
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"no name":         "steps:\n  - id: a\n    cmd: ls\n",
		"no steps":        "name: x\n",
		"dup id":          "name: x\nsteps:\n  - id: a\n    cmd: ls\n  - id: a\n    cmd: ls\n",
		"shell no cmd":    "name: x\nsteps:\n  - id: a\n    run: shell\n",
		"agent no prompt": "name: x\nsteps:\n  - id: a\n    run: agent\n",
	}
	for name, y := range cases {
		if _, err := recipe.Parse([]byte(y)); err == nil {
			t.Errorf("%s: expected a validation error", name)
		}
	}
}

func TestRunPipesOutputsThroughTemplate(t *testing.T) {
	r, err := recipe.Parse([]byte(pipelineYAML))
	if err != nil {
		t.Fatal(err)
	}
	fe := &fakeExec{shellOutput: func(string) string { return "3 files changed" }}
	runner := recipe.NewRunner(fe, true)

	res, err := runner.Run(context.Background(), r, nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Outputs["changes"]; got != "3 files changed" {
		t.Fatalf("shell output not recorded: %q", got)
	}
	if len(fe.agentCalls) != 1 || !strings.Contains(fe.agentCalls[0], "Summarize:\n3 files changed") {
		t.Fatalf("earlier step output not templated into prompt: %v", fe.agentCalls)
	}
	if !strings.Contains(fe.agentCalls[0], "backend|gemini|") {
		t.Fatalf("agent/provider not passed: %v", fe.agentCalls)
	}
	if len(res.Order) != 2 || res.Order[0] != "changes" || res.Order[1] != "summary" {
		t.Fatalf("steps ran out of order: %v", res.Order)
	}
}

func TestShellGatedWithoutAllowShell(t *testing.T) {
	r, _ := recipe.Parse([]byte("name: x\nsteps:\n  - id: a\n    run: shell\n    cmd: rm -rf /\n"))
	runner := recipe.NewRunner(&fakeExec{}, false)
	if _, err := runner.Run(context.Background(), r, nil, io.Discard); err == nil {
		t.Fatal("shell step must be refused without --allow-shell")
	}
}

func TestVarTemplating(t *testing.T) {
	r, _ := recipe.Parse([]byte("name: x\nsteps:\n  - id: a\n    run: agent\n    agent: backend\n    prompt: \"hi {{ var \\\"who\\\" }}\"\n"))
	fe := &fakeExec{}
	runner := recipe.NewRunner(fe, false)
	if _, err := runner.Run(context.Background(), r, map[string]string{"who": "world"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fe.agentCalls[0], "hi world") {
		t.Fatalf("var not substituted: %v", fe.agentCalls)
	}
}

func TestInferKind(t *testing.T) {
	// No explicit `run:`; kind inferred from fields.
	r, err := recipe.Parse([]byte("name: x\nsteps:\n  - id: a\n    cmd: ls\n  - id: b\n    agent: backend\n    prompt: hello\n"))
	if err != nil {
		t.Fatal(err)
	}
	fe := &fakeExec{}
	if _, err := recipe.NewRunner(fe, true).Run(context.Background(), r, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(fe.shellCmds) != 1 || len(fe.agentCalls) != 1 {
		t.Fatalf("kind inference wrong: shell=%v agent=%v", fe.shellCmds, fe.agentCalls)
	}
}
