package main

import (
	"os"
	"path/filepath"
	"strings"
)

// expandHome turns a leading ~ into the home directory, for paths the shell
// left alone (quoted, or after an = as in --dir=~/x).
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[1:])
}
