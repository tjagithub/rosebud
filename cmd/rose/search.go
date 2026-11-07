package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/store"
)

// runSearch prints the scenes of a bundle that match the words, best first,
// one line each: the time, then the translation, subtitle and spoken text.
func runSearch(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bundle := fs.String("bundle", "", "bundle directory to search (a FILE.film)")
	limit := fs.Int("limit", 10, "most scenes to show")
	if err := fs.Parse(args); err != nil {
		return err
	}
	words := strings.Join(fs.Args(), " ")
	if *bundle == "" || strings.TrimSpace(words) == "" || *limit <= 0 {
		return errors.New("usage: rose search --bundle DIR [--limit N] WORDS...")
	}
	dbPath := filepath.Join(*bundle, "rose.db")
	if err := statWithin(dbPath, 15*time.Second, os.Stat); err != nil {
		return fmt.Errorf("no catalog in %s: run `rose scenes` first", *bundle)
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
	hits, err := st.Search(ctx, core.SearchQuery{Text: words, Limit: *limit})
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
		if _, err := fmt.Fprintf(stdout, "%s  %s\n", clock(h.Scene.Start), strings.Join(parts, " | ")); err != nil {
			return err
		}
	}
	return nil
}

// clip cuts s to at most n runes, ending in an ellipsis when it cut.
func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// clock formats d as M:SS, or H:MM:SS from an hour on.
func clock(d time.Duration) string {
	s := int(d.Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s%3600/60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
