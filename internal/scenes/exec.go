package scenes

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Exec runs a program to completion and returns its stdout. It returns
// ctx's error if ctx ends first.
type Exec func(ctx context.Context, name string, args ...string) ([]byte, error)

// ExecOption configures NewExec.
type ExecOption func(*execConfig)

type execConfig struct{ waitDelay time.Duration }

// WithWaitDelay bounds how long to wait for output pipes after the program
// is killed, so an orphaned child holding a pipe cannot hang the caller.
func WithWaitDelay(d time.Duration) ExecOption {
	return func(c *execConfig) { c.waitDelay = d }
}

// stderrTail is how much of a failed program's stderr goes in the error.
const stderrTail = 2048

// NewExec returns an Exec backed by os/exec.
func NewExec(opts ...ExecOption) Exec {
	cfg := execConfig{waitDelay: 2 * time.Second}
	for _, opt := range opts {
		opt(&cfg)
	}
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		var stdout, stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		cmd.WaitDelay = cfg.waitDelay

		err := cmd.Run()
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s: %w", name, ctx.Err())
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w: %s", name, err, tail(stderr.String()))
		}
		return stdout.Bytes(), nil
	}
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > stderrTail {
		s = "..." + s[len(s)-stderrTail:]
	}
	return s
}
