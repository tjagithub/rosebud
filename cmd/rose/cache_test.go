package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRun_CacheClear(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ROSE_CACHE_DIR", dir)
	if err := os.MkdirAll(filepath.Join(dir, "ab"), 0o700); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(t.Context(), []string{"cache", "clear"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cache dir still there: %v", err)
	}
}

func TestRun_CacheUsage(t *testing.T) {
	for _, args := range [][]string{{"cache"}, {"cache", "wipe"}, {"cache", "clear", "x"}} {
		if err := run(t.Context(), args, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Errorf("%v: no error", args)
		}
	}
}
