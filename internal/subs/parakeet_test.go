package subs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/subs"
)

func TestParseParakeet(t *testing.T) {
	f, err := os.Open("testdata/parakeet_ja.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	got, err := subs.ParseParakeet(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 65 {
		t.Fatalf("%d tokens, want 65", len(got))
	}
	last := got[len(got)-1]
	if last.Text != "。" || last.Start != 59680*time.Millisecond || last.End != 59920*time.Millisecond {
		t.Errorf("last token = %+v", last)
	}
	if !strings.HasPrefix(subs.Assign([]subs.Span{{0, sec(61)}}, got)[0], "普通は") {
		t.Errorf("text does not start with 普通は")
	}
}

func TestParseParakeet_Edges(t *testing.T) {
	tests := []struct {
		name, in string
		want     []subs.Token
		bad      bool
	}{
		{"no sentences", `{"sentences":[]}`, nil, false},
		{"empty object", `{}`, nil, false},
		{"not json", `nope`, nil, true},
		{
			"tokens flatten across sentences",
			`{"sentences":[{"tokens":[{"text":"a","start":0,"end":1}]},{"tokens":[{"text":"b","start":1.5,"end":2}]}]}`,
			[]subs.Token{{"a", 0, sec(1)}, {"b", 1500 * time.Millisecond, sec(2)}},
			false,
		},
		{
			"inverted and negative tokens are dropped",
			`{"sentences":[{"tokens":[{"text":"x","start":2,"end":1},{"text":"y","start":-1,"end":1},{"text":"z","start":1,"end":2}]}]}`,
			[]subs.Token{{"z", sec(1), sec(2)}},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := subs.ParseParakeet(strings.NewReader(tt.in))
			if (err != nil) != tt.bad || !slices.Equal(got, tt.want) {
				t.Errorf("got %v, %v; want %v, bad=%v", got, err, tt.want, tt.bad)
			}
		})
	}
}

func TestModelFor(t *testing.T) {
	tests := []struct {
		lang, want string
		bad        bool
	}{
		{"ja", "mlx-community/parakeet-tdt_ctc-0.6b-ja", false},
		{"en", "mlx-community/parakeet-tdt-0.6b-v3", false},
		{"fr", "mlx-community/parakeet-tdt-0.6b-v3", false},
		{"uk", "mlx-community/parakeet-tdt-0.6b-v3", false},
		{"zh", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			got, err := subs.ModelFor(tt.lang)
			if got != tt.want || (err != nil) != tt.bad {
				t.Errorf("ModelFor(%q) = %q, %v; want %q, bad=%v", tt.lang, got, err, tt.want, tt.bad)
			}
		})
	}
}

// writeJSON fakes parakeet-mlx: it writes <wav base>.json into --output-dir.
func writeJSON(body string, calls *[]call) subs.Exec {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		*calls = append(*calls, call{name, args})
		dir := args[slices.Index(args, "--output-dir")+1]
		wav := args[slices.Index(args, "--model")-1]
		base := strings.TrimSuffix(filepath.Base(wav), filepath.Ext(wav))
		return nil, os.WriteFile(filepath.Join(dir, base+".json"), []byte(body), 0o644)
	}
}

func TestParakeet_Transcribe(t *testing.T) {
	var calls []call
	p := subs.Parakeet{Exec: writeJSON(`{"sentences":[{"tokens":[{"text":"a","start":0,"end":1}]}]}`, &calls), Model: "m/x"}
	got, err := p.Transcribe(t.Context(), "/w/00000000-00060000.wav")
	if err != nil {
		t.Fatal(err)
	}
	if want := []subs.Token{{"a", 0, sec(1)}}; !slices.Equal(got, want) {
		t.Errorf("tokens = %v, want %v", got, want)
	}
	a := calls[0].args
	if calls[0].name != "uvx" || !has(a, "--from", "parakeet-mlx==0.7.0", "parakeet-mlx", "/w/00000000-00060000.wav") ||
		!has(a, "--model", "m/x") || !has(a, "--output-format", "json") {
		t.Errorf("call = %v", calls[0])
	}
}

func TestParakeet_NoOutputFileIsAnError(t *testing.T) {
	p := subs.Parakeet{Exec: func(context.Context, string, ...string) ([]byte, error) { return nil, nil }, Model: "m"}
	if _, err := p.Transcribe(t.Context(), "a.wav"); err == nil {
		t.Error("want an error when no json was written")
	}
}

func TestParakeet_ExecError(t *testing.T) {
	boom := errors.New("boom")
	p := subs.Parakeet{Exec: func(context.Context, string, ...string) ([]byte, error) { return nil, boom }, Model: "m"}
	if _, err := p.Transcribe(t.Context(), "a.wav"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestParseParakeet_TimesAreExactMilliseconds(t *testing.T) {
	in := `{"sentences":[{"tokens":[{"text":"a","start":2.01,"end":2.03},{"text":"b","start":2.05,"end":2.07}]}]}`
	got, err := subs.ParseParakeet(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	if want := []subs.Token{{"a", ms(2010), ms(2030)}, {"b", ms(2050), ms(2070)}}; !slices.Equal(got, want) {
		t.Errorf("tokens = %v, want %v", got, want)
	}
}
