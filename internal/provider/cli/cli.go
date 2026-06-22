// Package cli implements a generic, config-driven Provider that runs an external
// agent CLI (Claude Code, Cursor Agent, agy, ...) as a runtime backend. The
// prompt is delivered either on stdin or as a command argument, so new CLIs are
// added by configuration, not code.
package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
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
}

// CmdProvider runs an agent CLI per its Spec.
type CmdProvider struct{ spec Spec }

// New builds a provider from spec.
func New(spec Spec) *CmdProvider { return &CmdProvider{spec: spec} }

func (p *CmdProvider) Name() string { return p.spec.Name }

// Streams reports whether Ask streams output to the terminal (vs. capturing it).
func (p *CmdProvider) Streams() bool { return p.spec.Stream }

// Available reports whether the CLI binary is resolvable on PATH.
func (p *CmdProvider) Available() bool {
	_, err := exec.LookPath(p.spec.Bin)
	return err == nil
}

// Ask invokes the CLI with the prompt and returns its captured stdout. In
// streaming mode the output is also echoed live to the terminal (tee), so
// callers get the text while the user sees progress. The prompt is delivered on
// stdin (Stdin mode) or as an argument; the process never inherits the parent's
// stdin, so an interactive caller (e.g. the chat REPL) keeps the keyboard.
func (p *CmdProvider) Ask(ctx context.Context, prompt string) (string, error) {
	if p.spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.spec.Timeout)
		defer cancel()
	}

	args := p.spec.Args
	if !p.spec.Stdin {
		args = withPrompt(args, prompt)
	}
	cmd := exec.CommandContext(ctx, p.spec.Bin, args...)
	if p.spec.Stdin {
		cmd.Stdin = strings.NewReader(prompt)
	}

	if p.spec.Format == "stream-json" {
		return p.askStreamJSON(cmd, os.Stdout)
	}

	var stdout, stderr bytes.Buffer
	if p.spec.Stream {
		cmd.Stdout = io.MultiWriter(os.Stdout, &stdout)
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return "", fmt.Errorf("%s: %w: %s", p.spec.Bin, err, msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// askStreamJSON runs the agent, renders its event stream to out as it arrives,
// and returns the final result text.
func (p *CmdProvider) askStreamJSON(cmd *exec.Cmd, out io.Writer) (string, error) {
	cmd.Stderr = os.Stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	result := renderStreamJSON(pipe, out)
	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("%s: %w", p.spec.Bin, err)
	}
	return strings.TrimSpace(result), nil
}

// streamEvent is the subset of Claude Code's stream-json events we render.
type streamEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Result  string `json:"result"`
	Message *struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	} `json:"message"`
}

// renderStreamJSON turns the newline-delimited event stream into readable live
// output (assistant text + dimmed tool-call lines) and returns the final result.
func renderStreamJSON(r io.Reader, out io.Writer) string {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	var final string
	for sc.Scan() {
		var e streamEvent
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue // ignore non-JSON / unknown lines
		}
		switch e.Type {
		case "assistant":
			if e.Message == nil {
				continue
			}
			for _, b := range e.Message.Content {
				switch b.Type {
				case "text":
					fmt.Fprint(out, b.Text)
				case "tool_use":
					fmt.Fprintf(out, "\n\x1b[2m· %s %s\x1b[0m\n", b.Name, toolSummary(b.Input))
				}
			}
		case "result":
			final = e.Result
		}
	}
	fmt.Fprintln(out)
	return final
}

// toolSummary extracts a short, human-readable hint from a tool's input.
func toolSummary(input json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(input, &m) != nil {
		return ""
	}
	for _, k := range []string{"file_path", "path", "command", "pattern", "url", "query"} {
		if v, ok := m[k].(string); ok && v != "" {
			if len(v) > 80 {
				v = v[:77] + "…"
			}
			return v
		}
	}
	return ""
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

// Builtins are the agent CLIs wired in by default. Workspace config can add to
// or override these by name.
func Builtins() []Spec {
	return []Spec{
		// claude renders its stream-json events live (text + tool calls).
		{Name: "claude", Bin: "claude", Args: []string{"-p", "--output-format", "stream-json", "--verbose"}, Stream: true, Format: "stream-json"},
		{Name: "cursor", Bin: "cursor-agent", Args: []string{"-p"}, Stream: true},
		{Name: "gemini", Bin: "gemini", Args: []string{"-p"}, Stream: true},
		{Name: "codex", Bin: "codex", Args: []string{"exec"}, Stream: true},
		// agy reads the prompt on stdin; its output still streams to the terminal.
		{Name: "agy", Bin: "agy", Args: []string{"-p"}, Stdin: true, Stream: true},
	}
}
