// Package recipe runs declarative, multi-step workflows. A recipe is an ordered
// list of steps; each step runs either a shell command or an agent, and a
// step's output can be templated into later steps — so a workflow like
// "pull → analyze → summarize → write commit message" is data, not code.
//
// The engine is deliberately decoupled from the provider/agent/shell layers via
// the Executor interface, so the orchestration logic stays pure and testable.
package recipe

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Step kinds.
const (
	KindShell = "shell"
	KindAgent = "agent"
)

// Step is one unit of a recipe.
type Step struct {
	ID string `yaml:"id"`
	// Run is the step kind: "shell" or "agent". When empty it is inferred from
	// the fields set (cmd → shell, prompt/agent → agent).
	Run string `yaml:"run"`
	// Cmd is the shell command (shell steps). Templated.
	Cmd string `yaml:"cmd"`
	// Agent/Provider/Prompt drive an agent step. Prompt is templated; Provider
	// is an optional per-step override.
	Agent    string `yaml:"agent"`
	Provider string `yaml:"provider"`
	Prompt   string `yaml:"prompt"`
}

// kind returns the resolved step kind, inferring it when Run is empty.
func (s Step) kind() string {
	if s.Run != "" {
		return s.Run
	}
	if s.Cmd != "" {
		return KindShell
	}
	return KindAgent
}

// Recipe is a named, ordered workflow.
type Recipe struct {
	Name  string `yaml:"name"`
	Steps []Step `yaml:"steps"`
}

// Parse decodes and validates a recipe from YAML.
func Parse(data []byte) (Recipe, error) {
	var r Recipe
	if err := yaml.Unmarshal(data, &r); err != nil {
		return Recipe{}, fmt.Errorf("parse recipe: %w", err)
	}
	if err := r.validate(); err != nil {
		return Recipe{}, err
	}
	return r, nil
}

// Load reads and validates a recipe file.
func Load(path string) (Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Recipe{}, err
	}
	return Parse(data)
}

// validate checks structural invariants so the runner can assume a clean recipe.
func (r Recipe) validate() error {
	if r.Name == "" {
		return fmt.Errorf("recipe has no name")
	}
	if len(r.Steps) == 0 {
		return fmt.Errorf("recipe %q has no steps", r.Name)
	}
	seen := map[string]bool{}
	for i, s := range r.Steps {
		if s.ID == "" {
			return fmt.Errorf("step %d has no id", i)
		}
		if seen[s.ID] {
			return fmt.Errorf("duplicate step id %q", s.ID)
		}
		seen[s.ID] = true

		switch s.kind() {
		case KindShell:
			if s.Cmd == "" {
				return fmt.Errorf("shell step %q has no cmd", s.ID)
			}
		case KindAgent:
			if s.Prompt == "" {
				return fmt.Errorf("agent step %q has no prompt", s.ID)
			}
		default:
			return fmt.Errorf("step %q has unknown run kind %q (want shell|agent)", s.ID, s.Run)
		}
	}
	return nil
}
