package translate_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/cache"
	"github.com/tjagithub/rosebud/internal/translate"
)

const model = "pfnet/plamo-3-translate-0.8b-MLX-4bit"

var plamoBackend = translate.Backends["plamo"]

func plamo(ex translate.Exec) translate.Local {
	return translate.Local{
		Exec: ex, Backend: plamoBackend, Model: model, Revision: plamoBackend.Revision, Source: "ja",
		Load: time.Minute, PerLine: time.Minute,
	}
}

// fakeUV records the command and the input it was given, then answers each line.
type fakeUV struct {
	args  []string
	input []map[string]any
	out   func(i int, text string) string
	err   error
	calls int
}

func (f *fakeUV) exec(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls++
	f.args = append([]string{name}, args...)
	in, _ := os.ReadFile(args[slices.IndexFunc(args, func(a string) bool { return strings.HasSuffix(a, ".py") })+1])
	f.input = nil
	var out strings.Builder
	out.WriteString("loading...\n")
	for _, ln := range strings.Split(strings.TrimSpace(string(in)), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(ln), &m) != nil {
			continue
		}
		f.input = append(f.input, m)
		out.WriteString(f.out(int(m["i"].(float64)), m["text"].(string)) + "\n")
	}
	return []byte(out.String()), f.err
}

func ok(i int, text string) string {
	b, _ := json.Marshal(map[string]any{"i": i, "text": "EN:" + text})
	return string(b)
}

func TestPlamoTranslatesAndSkipsEmpty(t *testing.T) {
	f := &fakeUV{out: ok}
	got, err := plamo(f.exec).Translate(t.Context(), []string{"はい", "", "うん"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"EN:はい", "", "EN:うん"}) {
		t.Errorf("got %q", got)
	}
	if len(f.input) != 2 {
		t.Errorf("sent %d lines, want 2", len(f.input))
	}
	for _, want := range []string{"uv", "run", "3.12", "mlx-lm==0.32.2", model, plamoBackend.Revision, "ja"} {
		if !slices.Contains(f.args, want) {
			t.Errorf("command lacks %q: %v", want, f.args)
		}
	}
}

func TestPlamoOnlyEmptyLinesRunsNothing(t *testing.T) {
	f := &fakeUV{out: ok}
	got, err := plamo(f.exec).Translate(t.Context(), []string{"", " "})
	if err != nil || f.calls != 0 || !slices.Equal(got, []string{"", ""}) {
		t.Errorf("got %q, %v, calls %d", got, err, f.calls)
	}
}

func TestPlamoErrors(t *testing.T) {
	cases := map[string]*fakeUV{
		"model error": {out: func(i int, _ string) string { return `{"i":0,"error":"boom"}` }},
		"missing line": {out: func(i int, _ string) string {
			if i == 0 {
				return ok(i, "x")
			}
			return "noise"
		}},
		"unknown line": {out: func(int, string) string { return `{"i":9,"text":"x"}` }},
		"exec fails":   {out: ok, err: errors.New("exit 1")},
	}
	for name, f := range cases {
		if _, err := plamo(f.exec).Translate(t.Context(), []string{"a", "b"}); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestPlamoBadConfig(t *testing.T) {
	p := plamo(nil)
	if _, err := p.Translate(t.Context(), []string{"a"}); err == nil {
		t.Error("nil exec: want an error")
	}
	f := &fakeUV{out: ok}
	p = plamo(f.exec)
	p.Source = ""
	if _, err := p.Translate(t.Context(), []string{"a"}); err == nil {
		t.Error("no source: want an error")
	}
}

func TestPlamoTimesOut(t *testing.T) {
	p := plamo(func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	p.Load, p.PerLine = 20*time.Millisecond, 20*time.Millisecond
	if _, err := p.Translate(t.Context(), []string{"a"}); err == nil {
		t.Error("want a timeout error")
	}
}

func TestCacheKeyChangesWithSetup(t *testing.T) {
	a, b := plamo(nil), plamo(nil)
	b.Source = "ko"
	c := plamo(nil)
	c.Revision = "other"
	if a.CacheKey() != plamo(nil).CacheKey() || a.CacheKey() == b.CacheKey() || a.CacheKey() == c.CacheKey() {
		t.Error("cache key must be stable and depend on source and revision")
	}
}

type fakeTr struct {
	calls [][]string
	err   error
}

func (f *fakeTr) Translate(_ context.Context, lines []string) ([]string, error) {
	f.calls = append(f.calls, slices.Clone(lines))
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = "EN:" + l
	}
	return out, f.err
}

func cached(f *fakeTr, dir string, batch int) translate.Cached {
	return translate.Cached{Inner: f, Dir: cache.Dir{Path: dir}, Salt: "s", Batch: batch}
}

func TestCachedDedupesBatchesAndReuses(t *testing.T) {
	dir := t.TempDir()
	f := &fakeTr{}
	var seen [][2]int64
	ctx := translate.WithProgress(t.Context(), func(d, tot int64) { seen = append(seen, [2]int64{d, tot}) })
	lines := []string{"a", "b", "a", "", "c"}
	got, err := cached(f, dir, 2).Translate(ctx, lines)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"EN:a", "EN:b", "EN:a", "", "EN:c"}) {
		t.Errorf("got %q", got)
	}
	if len(f.calls) != 2 || len(f.calls[0]) != 2 || len(f.calls[1]) != 1 {
		t.Errorf("calls = %v", f.calls)
	}
	if want := [][2]int64{{0, 3}, {2, 3}, {3, 3}}; !slices.Equal(seen, want) {
		t.Errorf("progress = %v, want %v", seen, want)
	}
	f2 := &fakeTr{}
	if _, err := cached(f2, dir, 2).Translate(t.Context(), lines); err != nil || len(f2.calls) != 0 {
		t.Errorf("second run: err %v, calls %v", err, f2.calls)
	}
}

func TestCachedKeepsFinishedBatchesOnFailure(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	fail := translatorFunc(func(ctx context.Context, l []string) ([]string, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("boom")
		}
		return []string{"EN:" + l[0]}, nil
	})
	c := translate.Cached{Inner: fail, Dir: cache.Dir{Path: dir}, Salt: "s", Batch: 1}
	if _, err := c.Translate(t.Context(), []string{"a", "b"}); err == nil {
		t.Fatal("want an error")
	}
	f := &fakeTr{}
	if _, err := cached(f, dir, 1).Translate(t.Context(), []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || f.calls[0][0] != "b" {
		t.Errorf("after failure only b should be redone, got %v", f.calls)
	}
}

func TestCachedSaltMissesAndBadConfig(t *testing.T) {
	dir := t.TempDir()
	f := &fakeTr{}
	_, _ = cached(f, dir, 5).Translate(t.Context(), []string{"a"})
	c := cached(f, dir, 5)
	c.Salt = "other"
	_, _ = c.Translate(t.Context(), []string{"a"})
	if len(f.calls) != 2 {
		t.Errorf("a new salt must miss, calls = %d", len(f.calls))
	}
	if _, err := cached(f, dir, 0).Translate(t.Context(), []string{"a"}); err == nil {
		t.Error("zero batch: want an error")
	}
}

type translatorFunc func(context.Context, []string) ([]string, error)

func (f translatorFunc) Translate(ctx context.Context, l []string) ([]string, error) {
	return f(ctx, l)
}

func TestBackendsArePinned(t *testing.T) {
	for name, b := range translate.Backends {
		if b.Name != name || len(b.Script) == 0 || b.Model == "" || len(b.Revision) != 40 {
			t.Errorf("backend %q is not fully pinned: %+v", name, b.Name)
		}
	}
}

func TestCacheKeyDiffersByBackend(t *testing.T) {
	a := plamo(nil)
	b := a
	b.Backend = translate.Backends["rikai"]
	if a.CacheKey() == b.CacheKey() {
		t.Error("backends must not share cache entries")
	}
}
