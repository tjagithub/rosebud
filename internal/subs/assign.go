package subs

import (
	"slices"
	"sort"
	"strings"
)

// Assign returns the text in each scene. A token belongs to the scene its Start
// falls in; others are dropped. Scenes must be sorted and disjoint.
func Assign(scenes []Span, tokens []Token) []string {
	toks := slices.Clone(tokens)
	slices.SortStableFunc(toks, func(a, b Token) int { return int(a.Start - b.Start) })

	parts := make([]strings.Builder, len(scenes))
	for _, t := range toks {
		i := sort.Search(len(scenes), func(i int) bool { return scenes[i].End > t.Start })
		if i < len(scenes) && t.Start >= scenes[i].Start {
			parts[i].WriteString(t.Text)
		}
	}
	out := make([]string, len(scenes))
	for i := range parts {
		out[i] = strings.TrimSpace(parts[i].String())
	}
	return out
}
