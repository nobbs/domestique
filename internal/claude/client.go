// Package claude runs the bundled claude executable for one prompt at a time,
// authenticated by the operator's Claude Code OAuth token. It knows nothing
// about rides: it takes a prompt and returns the text that answered it.
package claude

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// Model is the model every prompt is asked of, and what an answer names.
	Model = "claude-sonnet-5"

	defaultTimeout = 3 * time.Minute
	// maximumOutputBytes bounds the executable's JSON document, which carries
	// usage bookkeeping beside an answer of a few paragraphs.
	maximumOutputBytes = 1 << 20
	// waitDelay is how long a killed child's pipes may linger past the timeout.
	waitDelay = 5 * time.Second
)

// Category is a stable, safe-to-log reason a prompt produced no answer.
type Category string

const (
	// CategoryToken means the token was refused.
	CategoryToken Category = "token"
	// CategoryAllowance means the subscription's allowance is exhausted.
	CategoryAllowance Category = "allowance"
	// CategoryExecutable means the executable failed, timed out or answered
	// with something that is not its result document.
	CategoryExecutable Category = "executable"
	// CategoryUnusable means the answer was blank. A caller holding an answer to
	// a stricter bound reports its breach under the same category.
	CategoryUnusable Category = "unusable"
)

// Error is a failed prompt. It carries a category and never the prompt, the
// answer, or anything the executable printed.
type Error struct {
	cause    error
	Category Category
}

func (e *Error) Error() string { return "claude prompt failed: " + string(e.Category) }

// Unwrap exposes a context error, so a shutdown is distinguishable from a fault.
func (e *Error) Unwrap() error { return e.cause }

// Options configures a Client.
type Options struct {
	// Executable is the absolute path of the bundled claude executable.
	Executable string
	// Home is the absolute path of the writable directory the executable keeps
	// its configuration in, and the directory it runs in.
	Home string
	// Token is the operator's Claude Code OAuth token.
	Token []byte
	// Timeout bounds one prompt. Zero is three minutes.
	Timeout time.Duration
}

// Answer is the text one prompt was answered with.
type Answer struct {
	Text  string
	Model string
}

// Client runs one prompt per Ask.
type Client struct {
	executable string
	home       string
	token      string
	timeout    time.Duration
}

// New validates options without running anything.
func New(options Options) (*Client, error) {
	if !filepath.IsAbs(options.Executable) {
		return nil, errors.New("claude executable must be an absolute path")
	}
	if !filepath.IsAbs(options.Home) {
		return nil, errors.New("claude home must be an absolute path")
	}
	if len(options.Token) == 0 {
		return nil, errors.New("claude token is required")
	}
	if options.Timeout < 0 {
		return nil, errors.New("claude timeout must not be negative")
	}

	return &Client{
		executable: options.Executable,
		home:       options.Home,
		token:      string(options.Token),
		timeout:    cmp.Or(options.Timeout, defaultTimeout),
	}, nil
}

// result is the part of `--output-format json` this adapter reads.
type result struct {
	Type   string `json:"type"`
	Result string `json:"result"`
	//nolint:tagliatelle // Mirrors the executable's own field name.
	APIErrorStatus int `json:"api_error_status"`
	//nolint:tagliatelle // Mirrors the executable's own field name.
	IsError bool `json:"is_error"`
}

// Ask sends prompt on the child's standard input, where no process listing
// shows it, with every tool disabled and no session written to disk.
func (c *Client) Ask(ctx context.Context, prompt string) (Answer, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	//nolint:gosec // G204: the executable path is operator configuration; no argument is caller-supplied.
	command := exec.CommandContext(ctx, c.executable,
		"--print",
		"--output-format", "json",
		"--model", Model,
		"--tools", "",
		"--no-session-persistence",
		"--strict-mcp-config",
		// Home is a writable volume: a settings file planted there could add
		// hooks or point the token at another host, so none is read.
		"--setting-sources", "",
	)
	command.Dir = c.home
	command.Env = []string{
		"HOME=" + c.home,
		"CLAUDE_CODE_OAUTH_TOKEN=" + c.token,
		"DISABLE_AUTOUPDATER=1",
		"DISABLE_TELEMETRY=1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	}
	command.Stdin = strings.NewReader(prompt)
	output := &limitedBuffer{limit: maximumOutputBytes}
	command.Stdout = output
	command.WaitDelay = waitDelay

	runErr := command.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Answer{}, &Error{Category: CategoryExecutable, cause: ctxErr}
	}

	var document result
	if output.exceeded || json.Unmarshal(output.buffer.Bytes(), &document) != nil || document.Type != "result" {
		return Answer{}, &Error{Category: CategoryExecutable}
	}
	if document.IsError || runErr != nil {
		return Answer{}, &Error{Category: errorCategory(document.APIErrorStatus)}
	}
	text := strings.TrimSpace(document.Result)
	if text == "" {
		return Answer{}, &Error{Category: CategoryUnusable}
	}

	return Answer{Text: text, Model: Model}, nil
}

func errorCategory(status int) Category {
	switch status {
	case 401, 403:
		return CategoryToken
	case 429:
		return CategoryAllowance
	default:
		return CategoryExecutable
	}
}

// limitedBuffer keeps at most limit bytes and records that more were offered,
// accepting every write so the child is never blocked on a full pipe.
type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	kept := p
	if remaining := b.limit - b.buffer.Len(); len(kept) > remaining {
		b.exceeded = true
		kept = kept[:max(remaining, 0)]
	}
	if _, err := b.buffer.Write(kept); err != nil {
		return 0, fmt.Errorf("buffering claude output: %w", err)
	}

	return len(p), nil
}
