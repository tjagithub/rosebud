package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/tjagithub/rosebud/internal/ocr"
	"github.com/tjagithub/rosebud/internal/scenes"
	"github.com/tjagithub/rosebud/internal/shelf"
	"github.com/tjagithub/rosebud/internal/subload"
	"github.com/tjagithub/rosebud/internal/ui"
)

// runTUI opens the film library on a bundle.
func runTUI(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return execute(ctx, tuiCmd(), args, stdout, stderr)
}

func tuiCmd() *cobra.Command {
	var bundle, dir string
	var limit int
	cmd := &cobra.Command{
		Use:   "tui --bundle DIR | --dir DIR",
		Short: "Browse films and their scenes in the terminal",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			if (bundle == "") == (dir == "") || limit <= 0 {
				return fmt.Errorf("usage: %s", c.UseLine())
			}
			path := expandHome(bundle + dir)
			sh, err := shelf.Open(ctx, path)
			if err != nil {
				return err
			}
			defer func() { _ = sh.Close() }()
			subs, err := subtitleLoader(ctx, sh)
			if err != nil {
				return err
			}
			opts := []ui.Option{ui.WithBundle(expandHome(bundle)), ui.WithLibrary(sh), ui.WithSubtitles(subs)}
			_, err = tea.NewProgram(ui.New(sh, limit, opts...), tea.WithContext(ctx), tea.WithOutput(c.OutOrStdout())).Run()
			return err
		},
	}
	cmd.Flags().StringVar(&bundle, "bundle", "", "bundle directory to browse (a FILE.film)")
	cmd.Flags().IntVar(&limit, "limit", 30, "most hits to list in a search across films")
	cmd.Flags().StringVar(&dir, "dir", "", "directory holding FILE.film bundles to browse together")
	_ = cmd.MarkFlagDirname("bundle")
	_ = cmd.MarkFlagDirname("dir")
	return cmd
}

// subtitleLoader lets the scene screen load subtitles into the shelf's films,
// with the settings `rose ocr` uses. Nothing may print while the TUI runs.
func subtitleLoader(ctx context.Context, sh *shelf.Shelf) (subload.Loader, error) {
	cfg, err := loadConfig(ctx, "", os.Environ())
	if err != nil {
		return subload.Loader{}, err
	}
	ex := ocr.Exec(scenes.NewExec(scenes.WithWaitDelay(5 * time.Second)))
	timeout := cfg.Duration("OCR_TIMEOUT")
	return subload.Loader{
		Exec: ex, Store: sh.Store, Timeout: timeout,
		NewReader: func(bundle string) (ocr.Reader, error) {
			cfg, err := loadConfig(ctx, bundle, os.Environ())
			if err != nil {
				return nil, err
			}
			return nemotronReader(ctx, cfg, ex, cfg.Get("OCR_MODEL"), timeout, io.Discard, false)
		},
	}, nil
}
