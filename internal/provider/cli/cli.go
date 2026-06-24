// Package cli implements a generic, config-driven Provider that runs an external
// agent CLI (Claude Code, Cursor Agent, agy, ...) as a runtime backend. The
// prompt is delivered either on stdin or as a command argument, so new CLIs are
// added by configuration, not code.
package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/triforge-ai/aistack/internal/provider"
)

// promptPlaceholder, when present in Args, is replaced by the prompt. Otherwise,
// in argument mode, the prompt is appended as the final argument.
const promptPlaceholder = "{{prompt}}"

// Spec declares how to invoke an agent CLI.
type Spec struct {
	Name string
	Bin  string
	Args []string
	// Stdin pipes the prompt to the process's stdin (e.g. `agy -p`). When false,
	// the prompt is passed as an argument (e.g. `claude -p`, `cursor-agent -p`).
	Stdin bool
	// Stream echoes the agent's output to the terminal as it runs (the text is
	// still captured and returned).
	Stream bool
	// Format names the agent's output encoding. "stream-json" (Claude Code's
	// newline-delimited event stream) is rendered into readable live progress —
	// text and tool calls — while the final result is captured. Empty means
	// plain text.
	Format string
	// Timeout bounds a single invocation; 0 means no timeout.
	Timeout time.Duration
	// HealthArgs is the cheap liveness probe run by Health (e.g. ["--version"]).
	// It must not invoke the model. Empty defaults to ["--version"].
	HealthArgs []string
	// HealthTimeout bounds the liveness probe; 0 defaults to 10s.
	HealthTimeout time.Duration
	// WriteArgs are extra flags appended in write mode to let the agent actually
	// modify files / run tools (e.g. claude --permission-mode acceptEdits). Empty
	// means the provider has no known write toggle. Without these flags an agent
	// CLI typically runs read-only and silently discards edits.
	WriteArgs []string
}

// CmdProvider runs an agent CLI per its Spec.
type CmdProvider struct{ spec Spec }

// New builds a provider from spec.
func New(spec Spec) *CmdProvider { return &CmdProvider{spec: spec} }

func (p *CmdProvider) Name() string { return p.spec.Name }

// Streams reports whether Ask streams output to the terminal (vs. capturing it).
func (p *CmdProvider) Streams() bool { return p.spec.Stream }

// CanWrite reports whether this provider has write flags to enable (so callers
// can tell the user it is otherwise running read-only).
func (p *CmdProvider) CanWrite() bool { return len(p.spec.WriteArgs) > 0 }

// WithWrite returns a variant that appends the provider's WriteArgs, granting it
// permission to modify files / run tools. It returns the receiver unchanged when
// there are no write flags to apply.
func (p *CmdProvider) WithWrite() provider.Provider {
	if len(p.spec.WriteArgs) == 0 {
		return p
	}
	s := p.spec
	s.Args = append(append([]string{}, s.Args...), s.WriteArgs...)
	s.WriteArgs = nil // already folded into Args
	return &CmdProvider{spec: s}
}

// Available reports whether the CLI binary is resolvable on PATH.
func (p *CmdProvider) Available() bool {
	_, err := exec.LookPath(p.spec.Bin)
	return err == nil
}

// Health verifies the CLI is installed and actually runnable: it resolves the
// binary on PATH, then runs the cheap liveness probe (HealthArgs, default
// `--version`) with a short timeout. The probe must not call the model. On
// success it returns the probe's first output line (typically a version).
func (p *CmdProvider) Health(ctx context.Context) (string, error) {
	if _, err := exec.LookPath(p.spec.Bin); err != nil {
		return "", fmt.Errorf("not installed: %s not on PATH", p.spec.Bin)
	}

	probe := p.spec.HealthArgs
	if len(probe) == 0 {
		probe = []string{"--version"}
	}
	timeout := p.spec.HealthTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, p.spec.Bin, probe...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return "", fmt.Errorf("installed but not runnable (%s %s): %w: %s",
			p.spec.Bin, strings.Join(probe, " "), err, msg)
	}
	return firstLine(stdout.String()), nil
}

// firstLine returns the first non-empty line of s, trimmed.
func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			return t
		}
	}
	return ""
}

// Ask invokes the CLI with the prompt and returns its captured stdout. It is the
// back-compatible thin wrapper over Execute, which renders stream-json output
// live to the terminal by default. Callers that need token usage or a resumable
// session id call Execute directly.
func (p *CmdProvider) Ask(ctx context.Context, prompt string) (string, error) {
	res, err := p.Execute(ctx, prompt, provider.ExecOptions{})
	if err != nil {
		return "", err
	}
	return res.Output, nil
}

// Execute runs the prompt and returns a structured RunResult: the assistant
// text plus, for backends that report it, per-model token usage and a resumable
// session id. Typed progress events are delivered to opts.OnEvent as they
// arrive. This is the richer capability the simple Ask path is built on.
func (p *CmdProvider) Execute(ctx context.Context, prompt string, opts provider.ExecOptions) (provider.RunResult, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = p.spec.Timeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	spec := p.spec
	if opts.Write && len(spec.WriteArgs) > 0 {
		spec.Args = append(append([]string{}, spec.Args...), spec.WriteArgs...)
	}
	args := spec.Args
	if !spec.Stdin {
		args = withPrompt(args, prompt)
	}

	cmd := exec.CommandContext(ctx, spec.Bin, args...)
	cmd.Env = filteredEnv()
	if spec.Stdin {
		cmd.Stdin = strings.NewReader(prompt)
	}

	if spec.Format == "stream-json" {
		return p.executeStreamJSON(ctx, cmd, opts)
	}
	return p.executePlain(cmd, opts)
}

// executeStreamJSON drives a stream-json CLI: it pipes stdout through the typed
// parser (emitting opts.OnEvent), tees stderr to the terminal while retaining a
// bounded tail for error reporting, and assembles a RunResult with usage and the
// session id.
func (p *CmdProvider) executeStreamJSON(ctx context.Context, cmd *exec.Cmd, opts provider.ExecOptions) (provider.RunResult, error) {
	start := time.Now()
	stderr := newStderrTail(os.Stderr)
	cmd.Stderr = stderr

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return provider.RunResult{Status: "failed", Error: err.Error()}, err
	}
	if err := cmd.Start(); err != nil {
		return provider.RunResult{Status: "failed", Error: err.Error()}, fmt.Errorf("start %s: %w", p.spec.Bin, err)
	}

	// The provider only emits typed events; whether/how they reach a terminal is
	// the caller's renderer (opts.OnEvent). A nil sink simply captures silently.
	parsed := parseStreamJSON(pipe, opts.OnEvent)
	waitErr := cmd.Wait()

	res := provider.RunResult{
		Output:     parsed.text(),
		Status:     "completed",
		SessionID:  parsed.sessionID,
		DurationMs: time.Since(start).Milliseconds(),
		Usage:      parsed.usage,
	}
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		res.Status = "timeout"
		res.Error = withStderr(fmt.Sprintf("%s timed out", p.spec.Bin), stderr.Tail())
	case parsed.isError:
		res.Status = "failed"
		res.Error = withStderr(parsed.finalText, stderr.Tail())
	case waitErr != nil:
		res.Status = "failed"
		res.Error = withStderr(fmt.Sprintf("%s exited: %v", p.spec.Bin, waitErr), stderr.Tail())
	}
	if res.Error != "" {
		return res, fmt.Errorf("%s: %s", p.spec.Bin, res.Error)
	}
	return res, nil
}

// executePlain drives a CLI with no structured protocol: its stdout is opaque
// text. With a sink, stdout is streamed line-by-line as EventText so the caller
// renders it live; without one it is captured silently. Either way the provider
// never writes output to the terminal itself. Plain CLIs report no token usage,
// so Usage is left empty.
func (p *CmdProvider) executePlain(cmd *exec.Cmd, opts provider.ExecOptions) (provider.RunResult, error) {
	start := time.Now()
	var stdout bytes.Buffer
	stderr := newStderrTail(os.Stderr)
	cmd.Stderr = stderr

	var runErr error
	if opts.OnEvent != nil {
		pipe, err := cmd.StdoutPipe()
		if err != nil {
			return provider.RunResult{Status: "failed", Error: err.Error()}, err
		}
		if err := cmd.Start(); err != nil {
			return provider.RunResult{Status: "failed", Error: err.Error()}, fmt.Errorf("start %s: %w", p.spec.Bin, err)
		}
		sc := bufio.NewScanner(pipe)
		sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			stdout.WriteString(line)
			stdout.WriteByte('\n')
			opts.OnEvent(provider.Event{Kind: provider.EventText, Text: line + "\n"})
		}
		runErr = cmd.Wait()
	} else {
		cmd.Stdout = &stdout
		runErr = cmd.Run()
	}

	out := strings.TrimSpace(stdout.String())
	res := provider.RunResult{Output: out, Status: "completed", DurationMs: time.Since(start).Milliseconds()}
	if runErr != nil {
		detail := stderr.Tail()
		if detail == "" {
			detail = out
		}
		res.Status = "failed"
		res.Error = withStderr("", detail)
		return res, fmt.Errorf("%s: %w: %s", p.spec.Bin, runErr, detail)
	}
	return res, nil
}

// withPrompt substitutes the prompt for any placeholder argument, or appends it
// as the final argument when no placeholder is present.
func withPrompt(args []string, prompt string) []string {
	out := make([]string, 0, len(args)+1)
	replaced := false
	for _, a := range args {
		if a == promptPlaceholder {
			out = append(out, prompt)
			replaced = true
			continue
		}
		out = append(out, a)
	}
	if !replaced {
		out = append(out, prompt)
	}
	return out
}

// versionProbe is the standard liveness probe for the builtin CLIs: each
// supports `--version`, which runs the binary without invoking the model.
var versionProbe = []string{"--version"}

// Builtins are the agent CLIs wired in by default. Workspace config can add to
// or override these by name. Each declares its `ai health` liveness probe via
// HealthArgs; override it in workspace.yaml (`health_args:`) for a CLI whose
// version flag differs.
func Builtins() []Spec {
	return []Spec{
		// claude renders its stream-json events live (text + tool calls).
		{Name: "claude", Bin: "claude", Args: []string{"-p", "--output-format", "stream-json", "--verbose"}, Stream: true, Format: "stream-json", HealthArgs: versionProbe, WriteArgs: []string{"--permission-mode", "acceptEdits"}},
		{Name: "cursor", Bin: "cursor-agent", Args: []string{"-p"}, Stream: true, HealthArgs: versionProbe, WriteArgs: []string{"-f"}},
		{Name: "gemini", Bin: "gemini", Args: []string{"-p"}, Stream: true, HealthArgs: versionProbe, WriteArgs: []string{"--yolo"}},
		{Name: "codex", Bin: "codex", Args: []string{"exec"}, Stream: true, HealthArgs: versionProbe, WriteArgs: []string{"--full-auto"}},
		// agy reads the prompt on stdin; its output still streams to the terminal.
		{Name: "agy", Bin: "agy", Args: []string{"-p"}, Stdin: true, Stream: true, HealthArgs: versionProbe},
	}
}
