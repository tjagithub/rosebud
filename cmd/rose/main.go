// Command rose searches and browses films from the terminal.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rose:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		_, err := fmt.Fprintln(stdout, "rosebud")
		return err
	}
	switch args[0] {
	case "scenes":
		return runScenes(ctx, args[1:], stdout, stderr)
	case "subs":
		return runSubs(ctx, args[1:], stdout, stderr, parakeet)
	case "ocr":
		return runOCR(ctx, args[1:], stdout, stderr, nil)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
