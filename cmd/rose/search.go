package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/store"
	"github.com/tjagithub/rosebud/internal/ui"
)

// runSearch prints the scenes of a bundle that match the words.
func runSearch(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return execute(ctx, searchCmd(), args, stdout, stderr)
}

func searchCmd() *cobra.Command {
	var bundle string
	var limit int
	cmd := &cobra.Command{
		Use:     "search --bundle DIR WORDS...",
		Aliases: []string{"find", "s"},
		Short:   "Find the scenes in a bundle that match some words",
		Long:    "Prints one line per scene, best first: the time, then the translation, subtitle and spoken text.",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx, stdout := c.Context(), c.OutOrStdout()
			words := strings.Join(args, " ")
			if bundle == "" || strings.TrimSpace(words) == "" || limit <= 0 {
				return errors.New("usage: rose search --bundle DIR [--limit N] WORDS")
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
			hits, err := st.Search(ctx, core.SearchQuery{Text: words, Limit: limit})
			if err != nil {
				return err
			}
			for _, h := range hits {
				var parts []string
				for _, p := range []string{h.Translation, h.Subtitle, h.Text} {
					if p = strings.Join(strings.Fields(p), " "); p != "" {
						parts = append(parts, clip(p, 140))
					}
				}
				if _, err := fmt.Fprintf(stdout, "%s  %s\n", ui.Clock(h.Scene.Start), strings.Join(parts, " | ")); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&bundle, "bundle", "", "bundle directory to search (a FILE.film)")
	cmd.Flags().IntVar(&limit, "limit", 10, "most scenes to show")
	_ = cmd.MarkFlagDirname("bundle")
	return cmd
}

// clip cuts s to at most n runes, ending in an ellipsis when it cut.
func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
