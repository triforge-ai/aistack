package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/triforge-ai/aistack/internal/provider"
)

// checker is a fake provider that also reports health (like a CLI provider).
type checker struct {
	name      string
	detail    string
	healthErr error
	reply     string
	askErr    error
}

func (c checker) Name() string                                { return c.name }
func (c checker) Ask(context.Context, string) (string, error) { return c.reply, c.askErr }
func (c checker) Health(context.Context) (string, error)      { return c.detail, c.healthErr }

// plain is a provider with no health capability (like dryrun).
type plain struct{ name string }

func (p plain) Name() string                                { return p.name }
func (p plain) Ask(context.Context, string) (string, error) { return "", nil }

func TestCheckProviderHealthy(t *testing.T) {
	r := checkProvider(context.Background(), checker{name: "claude", detail: "v1.2.3"}, false)
	if !r.ok || r.detail != "v1.2.3" {
		t.Fatalf("want healthy with version detail, got %+v", r)
	}
}

func TestCheckProviderUnhealthy(t *testing.T) {
	r := checkProvider(context.Background(), checker{name: "codex", healthErr: errors.New("not installed: codex not on PATH")}, false)
	if r.ok {
		t.Fatalf("want unhealthy, got %+v", r)
	}
	if !strings.Contains(r.detail, "not installed") {
		t.Fatalf("detail should carry the error: %q", r.detail)
	}
}

func TestCheckProviderBuiltin(t *testing.T) {
	r := checkProvider(context.Background(), plain{name: "dryrun"}, false)
	if !r.ok || r.detail != "built-in" {
		t.Fatalf("non-checker should be built-in/healthy, got %+v", r)
	}
}

func TestCheckProviderLive(t *testing.T) {
	ctx := context.Background()

	ok := checkProvider(ctx, checker{name: "agy", detail: "v0.1", reply: "PONG"}, true)
	if !ok.ok || !strings.Contains(ok.detail, "responded") {
		t.Fatalf("live success should mark responded, got %+v", ok)
	}

	bad := checkProvider(ctx, checker{name: "agy", detail: "v0.1", askErr: errors.New("boom")}, true)
	if bad.ok || !strings.Contains(bad.detail, "live ping failed") {
		t.Fatalf("live ping error should be unhealthy, got %+v", bad)
	}

	empty := checkProvider(ctx, checker{name: "agy", detail: "v0.1", reply: "   "}, true)
	if empty.ok || !strings.Contains(empty.detail, "no output") {
		t.Fatalf("empty live reply should be unhealthy, got %+v", empty)
	}
}

func TestSelectProviders(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(checker{name: "claude"})
	reg.Register(plain{name: "dryrun"})

	all, err := selectProviders(reg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("no names should return all providers, got %d", len(all))
	}

	named, err := selectProviders(reg, []string{"claude"})
	if err != nil {
		t.Fatal(err)
	}
	if len(named) != 1 || named[0].Name() != "claude" {
		t.Fatalf("named selection wrong: %+v", named)
	}

	if _, err := selectProviders(reg, []string{"nope"}); err == nil {
		t.Fatal("unknown provider name should error")
	}
}
