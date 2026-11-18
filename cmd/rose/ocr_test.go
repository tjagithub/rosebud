package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/store"
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

func TestRun_OCRUsesTheStoredSpokenLanguageAndTheTrackFlag(t *testing.T) {
	file := makeTalkie(t)
	var out, errOut bytes.Buffer
	if err := run(t.Context(), []string{"scenes", file}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	dir, stem := filepath.Dir(file), strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	cue := func(text string) []byte { return []byte("1\n00:00:01,000 --> 00:00:02,000\n" + text + "\n") }
	for name, text := range map[string]string{stem + ".en.srt": "in english", stem + ".ja.srt": "in japanese", "elsewhere.srt": "from elsewhere"} {
		if err := os.WriteFile(filepath.Join(dir, name), cue(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	db, err := store.Open(filepath.Join(dir, stem+".film", "rose.db"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.New(t.Context(), db)
	if err == nil {
		err = st.SetSpoken(t.Context(), core.FilmID(stem), "ja")
	}
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	for args, want := range map[string]string{
		"--verbose": "jpn subrip from " + stem + ".ja.srt",
		"--verbose --track " + filepath.Join(dir, "elsewhere.srt"): "subrip from elsewhere.srt",
	} {
		errOut.Reset()
		if err := runOCR(t.Context(), append(strings.Fields(args), file), &out, &errOut, nil); err != nil {
			t.Fatalf("%s: %v\n%s", args, err, errOut.String())
		}
		if !strings.Contains(errOut.String(), "reading ") || !strings.Contains(errOut.String(), want) {
			t.Errorf("%s: stderr %q, want it to read %q", args, errOut.String(), want)
		}
	}
}

func TestRun_OCRHelpDocumentsTimeout(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := runOCR(t.Context(), []string{"-h"}, &out, &errOut, nil); err != nil {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{"--timeout", "--verbose", "--debug"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help lacks %s:\n%s", want, out.String())
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
