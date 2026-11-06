// Command rose searches and browses films from the terminal.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "rose:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	_ = args
	fmt.Println("rosebud")
	return nil
}
