package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/tjagithub/rosebud/internal/langid"
)

type fixedLang langid.Probs

func (f fixedLang) Windows(_ context.Context, wavs []string) ([]langid.Probs, error) {
	out := make([]langid.Probs, len(wavs))
	for i := range out {
		out[i] = langid.Probs(f)
	}
	return out, nil
}

func TestLang_NeedsAFile(t *testing.T) {
	var out, errb bytes.Buffer
	err := execute(t.Context(), langCmd(func(string) langid.Detector { return fixedLang{} }), nil, &out, &errb)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("err = %v", err)
	}
	err = execute(t.Context(), langCmd(func(string) langid.Detector { return fixedLang{} }), []string{"/no/such/file.mkv"}, &out, &errb)
	if err == nil {
		t.Error("missing file: no error")
	}
}
