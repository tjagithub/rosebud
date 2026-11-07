package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// runCache implements `rose cache clear`, which deletes cached results.
func runCache(ctx context.Context, args []string, stdout, _ io.Writer) error {
	if len(args) != 1 || args[0] != "clear" {
		return errors.New("usage: rose cache clear")
	}
	cfg, err := loadConfig(ctx, "", os.Environ())
	if err != nil {
		return err
	}
	dir, err := cacheDir(cfg)
	if err != nil {
		return err
	}
	if err := removeWithin(dir, 30*time.Second); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "cleared %s\n", dir)
	return err
}

// removeWithin deletes dir, giving up after d in case the disk stalls.
func removeWithin(dir string, d time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- os.RemoveAll(dir) }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		return fmt.Errorf("clearing %s took over %v", dir, d)
	}
}
