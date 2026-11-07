package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/tjagithub/rosebud/internal/ocr"
	"github.com/tjagithub/rosebud/internal/subs"
)

// deps are the parts of the commands a test can replace; nil picks the real one.
type deps struct {
	transcriber func(model string) subs.Transcriber
	reader      func(model string, timeout time.Duration) ocr.Reader
}

// newRoot builds the `rose` command with every subcommand.
func newRoot(d deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "rose",
		Short:         "Search and browse your films from the terminal",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	tr := d.transcriber
	if tr == nil {
		tr = parakeet
	}
	root.AddCommand(scenesCmd(), subsCmd(tr), ocrCmd(d.reader), translateCmd(nil), searchCmd(), tuiCmd(), cacheCmd())
	return root
}

// execute runs one command with args and the given streams, for tests and the
// run helpers; errors come back unprinted.
func execute(ctx context.Context, cmd *cobra.Command, args []string, stdout, stderr io.Writer) error {
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	return cmd.ExecuteContext(ctx)
}

// oneFile requires exactly one FILE argument and says how to call the command.
func oneFile(c *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: %s", c.UseLine())
	}
	return nil
}
