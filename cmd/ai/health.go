package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/triforge-ai/aistack/internal/provider"
)

// pingPrompt is the trivial message sent during a --live health check. It must
// stay cheap and side-effect free.
const pingPrompt = "Health check: reply with the single word PONG."

// livePingTimeout bounds an end-to-end --live ping per provider.
const livePingTimeout = 60 * time.Second

// cmdHealth probes provider CLIs and reports whether each is installed and
// runnable. With no names it checks every registered provider; otherwise only
// the named ones (e.g. `ai health claude agy codex`). With --live it also sends
// a tiny prompt end-to-end to verify the agent actually answers (this invokes
// the real agent and may cost tokens). It exits non-zero if any are unhealthy.
func cmdHealth(args []string) error {
	live := false
	var names []string
	for _, a := range args {
		switch {
		case a == "--live":
			live = true
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown flag %q (usage: ai health [provider...] [--live])", a)
		default:
			names = append(names, a)
		}
	}

	a, _, err := openWorkspace()
	if err != nil {
		return err
	}
	providers, err := selectProviders(a.Provider, names)
	if err != nil {
		return err
	}

	ctx := context.Background()
	failed := 0
	for _, p := range providers {
		if live {
			fmt.Printf("→ pinging %s …\n", p.Name())
		}
		r := checkProvider(ctx, p, live)
		mark := "✓"
		if !r.ok {
			mark = "✗"
			failed++
		}
		fmt.Printf("%s  %-10s %s\n", mark, r.name, r.detail)
	}
	if failed > 0 {
		return fmt.Errorf("%d/%d provider(s) unhealthy", failed, len(providers))
	}
	return nil
}

// selectProviders returns all registered providers when names is empty, or just
// the named ones (erroring on the first unknown name).
func selectProviders(reg *provider.Registry, names []string) ([]provider.Provider, error) {
	if len(names) == 0 {
		return reg.All(), nil
	}
	out := make([]provider.Provider, 0, len(names))
	for _, n := range names {
		p, err := reg.Get(n)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// healthResult is one provider's health status.
type healthResult struct {
	name   string
	ok     bool
	detail string
}

// checkProvider runs the liveness probe for a single provider. Providers without
// an external binary (e.g. dryrun) are reported as built-in/healthy. When live
// is set and the probe passes, it additionally sends pingPrompt end-to-end.
func checkProvider(ctx context.Context, p provider.Provider, live bool) healthResult {
	res := healthResult{name: p.Name()}

	hc, ok := p.(provider.HealthChecker)
	if !ok {
		res.ok = true
		res.detail = "built-in"
		return res
	}

	detail, err := hc.Health(ctx)
	if err != nil {
		res.detail = err.Error()
		return res
	}
	res.ok = true
	res.detail = detail

	if live {
		pingCtx, cancel := context.WithTimeout(ctx, livePingTimeout)
		defer cancel()
		reply, err := p.Ask(pingCtx, pingPrompt)
		switch {
		case err != nil:
			res.ok = false
			res.detail = "live ping failed: " + err.Error()
		case strings.TrimSpace(reply) == "":
			res.ok = false
			res.detail = "live ping returned no output"
		default:
			res.detail = strings.TrimSpace(detail + " · responded")
		}
	}
	return res
}
