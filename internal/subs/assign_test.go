package subs_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/tjagithub/rosebud/internal/subs"
)

func tok(text string, startMs int) subs.Token {
	d := time.Duration(startMs) * time.Millisecond
	return subs.Token{Text: text, Start: d, End: d + 100*time.Millisecond}
}

func TestAssign(t *testing.T) {
	scenes := []subs.Span{{0, sec(10)}, {sec(10), sec(20)}, {sec(20), sec(30)}}
	tests := []struct {
		name   string
		scenes []subs.Span
		tokens []subs.Token
		want   []string
	}{
		{"no tokens", scenes, nil, []string{"", "", ""}},
		{"no scenes", nil, []subs.Token{tok("a", 0)}, []string{}},
		{
			"tokens join without spaces", scenes,
			[]subs.Token{tok("村", 100), tok("川", 200), tok("さん", 300)},
			[]string{"村川さん", "", ""},
		},
		{
			"separator tokens are kept inside and trimmed at the ends", scenes,
			[]subs.Token{tok(" ", 0), tok("is", 100), tok(" ", 200), tok("it", 300), tok(" ", 400)},
			[]string{"is it", "", ""},
		},
		{
			"a token belongs to the scene it starts in", scenes,
			[]subs.Token{tok("a", 9_999), tok("b", 10_000), tok("c", 19_999)},
			[]string{"a", "bc", ""},
		},
		{
			"a token spanning a cut stays in its start scene", scenes,
			[]subs.Token{{Text: "long", Start: sec(9), End: sec(12)}},
			[]string{"long", "", ""},
		},
		{
			"tokens outside every scene are dropped", scenes,
			[]subs.Token{tok("x", -500), tok("y", 30_000), tok("z", 99_000)},
			[]string{"", "", ""},
		},
		{
			"tokens are ordered by time not input order", scenes,
			[]subs.Token{tok("b", 200), tok("a", 100)},
			[]string{"ab", "", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := subs.Assign(tt.scenes, tt.tokens)
			if !slices.Equal(got, tt.want) {
				t.Errorf("Assign = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAssign_Properties(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		cuts := rapid.SliceOfN(rapid.IntRange(1, 99), 0, 8).Draw(rt, "cuts")
		slices.Sort(cuts)
		cuts = slices.Compact(cuts)
		bounds := append(append([]int{0}, cuts...), 100)
		var scenes []subs.Span
		for i := 1; i < len(bounds); i++ {
			scenes = append(scenes, subs.Span{Start: sec(bounds[i-1]), End: sec(bounds[i])})
		}
		// Letters are unique so each token can be found in the output.
		n := rapid.IntRange(0, 26).Draw(rt, "n")
		tokens := make([]subs.Token, n)
		for i := range tokens {
			tokens[i] = tok(string(rune('a'+i)), rapid.IntRange(-2_000, 102_000).Draw(rt, "ms"))
		}

		got := subs.Assign(scenes, slices.Clone(tokens))

		if len(got) != len(scenes) {
			rt.Fatalf("got %d texts for %d scenes", len(got), len(scenes))
		}
		for _, tk := range tokens {
			want := -1
			for i, s := range scenes {
				if tk.Start >= s.Start && tk.Start < s.End {
					want = i
				}
			}
			for i, text := range got {
				if has := strings.Contains(text, tk.Text); has != (i == want) {
					rt.Fatalf("token %q at %v: in scene %d = %v, want scene %d", tk.Text, tk.Start, i, has, want)
				}
			}
		}
		for _, text := range got {
			if text != strings.TrimSpace(text) {
				rt.Fatalf("text %q is not trimmed", text)
			}
		}
	})
}
