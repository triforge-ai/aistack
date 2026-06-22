// Package provider is the replaceable runtime layer. A Provider executes a
// prompt against some backend (Claude CLI, Codex, Gemini, an HTTP API, ...).
// Agents own the logic; providers are interchangeable execution engines.
package provider

import (
	"context"
	"fmt"
	"sort"
)

// Provider runs prompts against a backend.
type Provider interface {
	Name() string
	Ask(ctx context.Context, prompt string) (string, error)
}

// Available is an optional capability: providers backed by an external binary
// can report whether that binary is installed.
type Available interface {
	Available() bool
}

// Streamer is an optional capability: providers that write their output
// directly to the terminal (rather than returning it) report it here so callers
// don't double-print.
type Streamer interface {
	Streams() bool
}

// HealthChecker is an optional capability: a provider can verify it is not just
// installed but actually runnable, returning a short detail (e.g. a version
// string) on success or an error describing why it is unhealthy.
type HealthChecker interface {
	Health(ctx context.Context) (detail string, err error)
}

// Registry holds the available providers by name.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{providers: map[string]Provider{}}
}

// Register adds a provider, keyed by its Name().
func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
}

// Get returns the named provider.
func (r *Registry) Get(name string) (Provider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q (have: %v)", name, r.Names())
	}
	return p, nil
}

// Has reports whether a provider is registered under name.
func (r *Registry) Has(name string) bool {
	_, ok := r.providers[name]
	return ok
}

// Names lists registered provider names, sorted.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.providers))
	for n := range r.providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// All returns the registered providers ordered by name.
func (r *Registry) All() []Provider {
	out := make([]Provider, 0, len(r.providers))
	for _, n := range r.Names() {
		out = append(out, r.providers[n])
	}
	return out
}
