package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/config"
)

func TestParse(t *testing.T) {
	in := "# a comment\n\nOCR_MODEL = \"some/model\"\nexport OCR_LANG=ja # trailing\nNAME='it''s'\nQ=\"a \\\"b\\\"\"\nEMPTY=\nOCR_LANG=en\r\n"
	got, err := config.Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"OCR_MODEL": "some/model", "OCR_LANG": "en", "NAME": "it''s", "Q": `a "b"`, "EMPTY": ""}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestParseRejectsBadLines(t *testing.T) {
	for in, want := range map[string]string{
		"no equals sign\n": "line 1",
		"ok=1\n":           "line 1", // lowercase key
		"A=1\n=2\n":        "line 2",
		"A B=1\n":          "line 1",
	} {
		_, err := config.Parse(strings.NewReader(in))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want mention of %q", in, err, want)
		}
	}
}

var schema = config.Schema{
	{Name: "OCR_MODEL", Default: "m", Doc: "OCR model"},
	{Name: "OCR_TIMEOUT", Default: "30m", Doc: "longest OCR run", Check: config.IsDuration},
	{Name: "OCR_RETRIES", Default: "2", Check: config.IsInt},
}

func TestLoadLayersLaterWins(t *testing.T) {
	c, err := schema.Load(
		config.Layer{Name: "user", Values: map[string]string{"OCR_MODEL": "u", "OCR_RETRIES": "5"}},
		config.Layer{Name: "film", Values: map[string]string{"OCR_MODEL": "f"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if c.Get("OCR_MODEL") != "f" || c.Int("OCR_RETRIES") != 5 || c.Duration("OCR_TIMEOUT") != 30*time.Minute {
		t.Errorf("got %q %d %v", c.Get("OCR_MODEL"), c.Int("OCR_RETRIES"), c.Duration("OCR_TIMEOUT"))
	}
}

func TestLoadRejects(t *testing.T) {
	for name, l := range map[string]config.Layer{
		"unknown key":  {Name: "user", Values: map[string]string{"OCR_MODLE": "x"}},
		"bad duration": {Name: "film", Values: map[string]string{"OCR_TIMEOUT": "soon"}},
		"zero timeout": {Name: "film", Values: map[string]string{"OCR_TIMEOUT": "0s"}},
		"bad int":      {Name: "film", Values: map[string]string{"OCR_RETRIES": "two"}},
	} {
		_, err := schema.Load(l)
		if err == nil || !strings.Contains(err.Error(), l.Name) {
			t.Errorf("%s: err = %v, want it to name the layer %q", name, err, l.Name)
		}
	}
}

func TestLooseLayerSkipsUnknownKeys(t *testing.T) {
	c, err := schema.Load(config.Layer{Name: "env", Loose: true, Values: map[string]string{"PATH": "x", "OCR_MODEL": "e"}})
	if err != nil || c.Get("OCR_MODEL") != "e" {
		t.Errorf("err %v, model %q", err, c.Get("OCR_MODEL"))
	}
}

func TestEnvLayer(t *testing.T) {
	l := config.Env([]string{"ROSE_OCR_MODEL=e", "OCR_MODEL=ignored", "ROSE_=x", "HOME=/h", "ROSE_NOEQUALS"})
	if !l.Loose || len(l.Values) != 1 || l.Values["OCR_MODEL"] != "e" {
		t.Errorf("layer = %+v", l)
	}
}

func TestFileLayer(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "rose.conf")
	if l, err := config.File(t.Context(), p, time.Second); err != nil || len(l.Values) != 0 {
		t.Errorf("missing file: %+v, %v", l, err)
	}
	if err := os.WriteFile(p, []byte("OCR_MODEL=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := config.File(t.Context(), p, time.Second)
	if err != nil || l.Values["OCR_MODEL"] != "x" || l.Name != p {
		t.Errorf("%+v, %v", l, err)
	}
	if err := os.WriteFile(p, []byte("junk\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.File(t.Context(), p, time.Second); err == nil || !strings.Contains(err.Error(), p) {
		t.Errorf("bad file: err = %v, want the path named", err)
	}
	if _, err := config.File(t.Context(), p, 0); err == nil {
		t.Error("zero timeout: no error")
	}
}

func TestIsPositiveInt(t *testing.T) {
	for v, ok := range map[string]bool{"1": true, "500": true, "0": false, "-3": false, "x": false, "": false} {
		if got := config.IsPositiveInt(v) == nil; got != ok {
			t.Errorf("IsPositiveInt(%q) ok = %t, want %t", v, got, ok)
		}
	}
}

func TestOneOf(t *testing.T) {
	check := config.OneOf("auto", "none")
	if check("auto") != nil || check("none") != nil || check("gpu") == nil || check("") == nil {
		t.Error("OneOf accepted or rejected the wrong values")
	}
}
