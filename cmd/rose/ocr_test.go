package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRun_OCRRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.mkv")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"no file":        {},
		"missing file":   {filepath.Join(dir, "nope.mkv")},
		"no scenes yet":  {file},
		"zero timeout":   {"--timeout", "0s", file},
		"bad timeout":    {"--timeout", "soon", file},
		"empty language": {"--lang", "", file},
	} {
		var out, errOut bytes.Buffer
		if err := runOCR(t.Context(), args, &out, &errOut, nil); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestRun_OCRHelpDocumentsTimeout(t *testing.T) {
	var out, errOut bytes.Buffer
	err := runOCR(t.Context(), []string{"-h"}, &out, &errOut, nil)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("err = %v, want ErrHelp", err)
	}
	for _, want := range []string{"--timeout", "--verbose", "--debug"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("help lacks %s:\n%s", want, errOut.String())
		}
	}
}

func TestStatWithinGivesUpOnAStall(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	stall := func(string) (os.FileInfo, error) { <-release; return nil, nil }
	start := time.Now()
	if err := statWithin("/x", 50*time.Millisecond, stall); err == nil || !strings.Contains(err.Error(), "took over") {
		t.Errorf("err = %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("waited far longer than the limit")
	}
}
