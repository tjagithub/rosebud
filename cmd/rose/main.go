// Command rose searches and browses films from the terminal.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"charm.land/fang/v2"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := fang.Execute(ctx, newRoot(deps{}), fang.WithNotifySignal(os.Interrupt, syscall.SIGTERM))
	stop()
	if err != nil {
		os.Exit(1)
	}
}

// run executes one rose command line without fang's styling, for tests.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		_, err := fmt.Fprintln(stdout, "rosebud")
		return err
	}
	return execute(ctx, newRoot(deps{}), args, stdout, stderr)
}
