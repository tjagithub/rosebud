package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHomeResolvesTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{
		"~":            home,
		"~/dock/Films": filepath.Join(home, "dock/Films"),
		"/abs/path":    "/abs/path",
		"rel/~/x":      "rel/~/x",
		"~other/x":     "~other/x",
		"":             "",
	} {
		if got := expandHome(in); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}
