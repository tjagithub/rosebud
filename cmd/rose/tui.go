package main

import (
	"context"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/tjagithub/rosebud/internal/shelf"
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
			path := bundle + dir
			sh, err := shelf.Open(ctx, path)
			if err != nil {
				return err
			}
			defer func() { _ = sh.Close() }()
			_, err = tea.NewProgram(ui.New(sh, limit, ui.WithBundle(bundle), ui.WithLibrary(sh)), tea.WithContext(ctx), tea.WithOutput(c.OutOrStdout())).Run()
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
