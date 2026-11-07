package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"
)

// Env makes a Loose layer from environ lines such as ROSE_OCR_MODEL=x, which
// set OCR_MODEL. Other variables are ignored.
func Env(environ []string) Layer {
	l := Layer{Name: "environment", Loose: true, Values: map[string]string{}}
	for _, e := range environ {
		k, v, ok := strings.Cut(e, "=")
		name, found := strings.CutPrefix(k, "ROSE_")
		if ok && found && name != "" {
			l.Values[name] = v
		}
	}
	return l
}

// File reads a config file as a layer. A missing file is an empty layer. It
// gives up after timeout, which must be positive, in case a mount stalls.
func File(ctx context.Context, path string, timeout time.Duration) (Layer, error) {
	if timeout <= 0 {
		return Layer{}, fmt.Errorf("config: reading %s: timeout must be positive, got %v", path, timeout)
	}
	type result struct {
		b   []byte
		err error
	}
	done := make(chan result, 1)
	go func() { b, err := os.ReadFile(path); done <- result{b, err} }()
	var r result
	select {
	case r = <-done:
	case <-time.After(timeout):
		return Layer{}, fmt.Errorf("config: reading %s took over %v", path, timeout)
	case <-ctx.Done():
		return Layer{}, ctx.Err()
	}
	if errors.Is(r.err, fs.ErrNotExist) {
		return Layer{Name: path}, nil
	}
	if r.err != nil {
		return Layer{}, fmt.Errorf("config: %w", r.err)
	}
	vals, err := Parse(bytes.NewReader(r.b))
	if err != nil {
		return Layer{}, fmt.Errorf("%s: %w", path, err)
	}
	return Layer{Name: path, Values: vals}, nil
}
