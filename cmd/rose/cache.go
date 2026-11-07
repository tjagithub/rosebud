package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"os"
	"time"
)

// runCache implements `rose cache clear`, which deletes cached results.
func runCache(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return execute(ctx, cacheCmd(), args, stdout, stderr)
}

func cacheCmd() *cobra.Command {
	cache := &cobra.Command{
		Use:   "cache",
		Short: "Manage cached results",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return errors.New("usage: rose cache clear") },
	}
	cache.AddCommand(&cobra.Command{
		Use:   "clear",
		Short: "Delete cached results",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, err := loadConfig(c.Context(), "", os.Environ())
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
			_, err = fmt.Fprintf(c.OutOrStdout(), "cleared %s\n", dir)
			return err
		},
	})
	return cache
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
