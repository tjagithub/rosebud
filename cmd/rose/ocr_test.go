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

func TestLoadConfigLayers(t *testing.T) {
	bundle := t.TempDir()
	if err := os.WriteFile(filepath.Join(bundle, "rose.conf"), []byte("OCR_MODEL=film/model\nOCR_LANG=en\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	c, err := loadConfig(t.Context(), bundle, []string{"ROSE_OCR_LANG=fr"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Get("OCR_MODEL") != "film/model" || c.Get("OCR_LANG") != "fr" || c.Duration("OCR_TIMEOUT") != 30*time.Minute {
		t.Errorf("model %q lang %q timeout %v", c.Get("OCR_MODEL"), c.Get("OCR_LANG"), c.Duration("OCR_TIMEOUT"))
	}
	if err := os.WriteFile(filepath.Join(bundle, "rose.conf"), []byte("OCR_MODLE=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(t.Context(), bundle, nil); err == nil {
		t.Error("typo in a key: no error")
	}
}

func TestLoadConfigWithoutBundleIgnoresTheWorkingDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	if err := os.WriteFile("rose.conf", []byte("OCR_MODEL=stray\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(t.Context(), "", nil)
	if err != nil || c.Get("OCR_MODEL") == "stray" {
		t.Errorf("model %q, err %v", c.Get("OCR_MODEL"), err)
	}
}

func TestHWAccelFor(t *testing.T) {
	for _, c := range []struct{ setting, goos, want string }{
		{"auto", "darwin", "videotoolbox"}, {"auto", "linux", "auto"}, {"cuda", "linux", "cuda"}, {"cuda", "darwin", "cuda"}, {"none", "darwin", ""}, {"", "darwin", ""},
	} {
		if got := hwaccelFor(c.setting, c.goos); got != c.want {
			t.Errorf("hwaccelFor(%q, %q) = %q, want %q", c.setting, c.goos, got, c.want)
		}
	}
}

func TestIsHWAccelName(t *testing.T) {
	for v, ok := range map[string]bool{"auto": true, "none": true, "cuda": true, "d3d11va": true, "videotoolbox": true, "": false, "CUDA": false, "a b": false, "../x": false} {
		if got := isHWAccelName(v) == nil; got != ok {
			t.Errorf("isHWAccelName(%q) ok = %t, want %t", v, got, ok)
		}
	}
}
