package langid_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/langid"
)

func TestWhisper_Windows(t *testing.T) {
	var gotArgs []string
	ex := func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)
		return []byte("warning on stderr-ish line\n[{\"ja\":0.9,\"en\":0.1},null]\n"), nil
	}
	w := langid.Whisper{Exec: ex, Model: "openai/whisper-tiny", Load: time.Minute, PerWindow: time.Second}
	got, err := w.Windows(t.Context(), []string{"a.wav", "b.wav"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0]["ja"] != 0.9 || got[1] != nil {
		t.Errorf("got %v", got)
	}
	if gotArgs[0] != "uv" || !slices.Contains(gotArgs, "openai/whisper-tiny") || !slices.Contains(gotArgs, "b.wav") {
		t.Errorf("args = %v", gotArgs)
	}
}

func TestWhisper_Errors(t *testing.T) {
	boom := errors.New("boom")
	cases := map[string]langid.Whisper{
		"exec":   {Exec: func(context.Context, string, ...string) ([]byte, error) { return nil, boom }, Model: "m", Load: time.Minute, PerWindow: time.Second},
		"json":   {Exec: func(context.Context, string, ...string) ([]byte, error) { return []byte("nope"), nil }, Model: "m", Load: time.Minute, PerWindow: time.Second},
		"count":  {Exec: func(context.Context, string, ...string) ([]byte, error) { return []byte("[null]"), nil }, Model: "m", Load: time.Minute, PerWindow: time.Second},
		"config": {Model: "m"},
	}
	for name, w := range cases {
		if _, err := w.Windows(t.Context(), []string{"a.wav", "b.wav"}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
