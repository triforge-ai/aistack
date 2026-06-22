// Package dryrun provides a Provider that echoes the assembled prompt instead
// of calling a model. It makes the full pipeline runnable and testable with no
// external CLI or API key.
package dryrun

import "context"

// Provider returns the prompt back, prefixed, so users can inspect exactly what
// the context builder produced.
type Provider struct{}

// New returns a dry-run provider.
func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "dryrun" }

func (p *Provider) Ask(_ context.Context, prompt string) (string, error) {
	return "----- DRY RUN (assembled prompt) -----\n" + prompt, nil
}
