package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/tjagithub/rosebud/internal/engine"
	"github.com/tjagithub/rosebud/internal/store"
	"github.com/tjagithub/rosebud/internal/ui"
)

// runTUI opens the search screen on a bundle.
func runTUI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return execute(ctx, tuiCmd(), args, stdout, stderr)
}

func tuiCmd() *cobra.Command {
	var bundle string
	var limit int
	cmd := &cobra.Command{
		Use:   "tui --bundle DIR",
		Short: "Browse a film's scenes in the terminal",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			if bundle == "" || limit <= 0 {
				return fmt.Errorf("usage: %s", c.UseLine())
			}
			dbPath := filepath.Join(bundle, "rose.db")
			if err := statWithin(dbPath, 15*time.Second, os.Stat); err != nil {
				return fmt.Errorf("no catalog in %s: run `rose scenes` first", bundle)
			}
			db, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()
			st, err := store.New(ctx, db)
			if err != nil {
				return err
			}
			eng := engine.New(st)
			defer func() { _ = eng.Close() }()
			_, err = tea.NewProgram(ui.New(eng, limit), tea.WithContext(ctx), tea.WithOutput(c.OutOrStdout())).Run()
			return err
		},
	}
	cmd.Flags().StringVar(&bundle, "bundle", "", "bundle directory to browse (a FILE.film)")
	cmd.Flags().IntVar(&limit, "limit", 30, "most scenes to list")
	_ = cmd.MarkFlagDirname("bundle")
	return cmd
}
