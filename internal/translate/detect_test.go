package translate_test

import (
	"testing"

	"github.com/tjagithub/rosebud/internal/translate"
)

func TestDetectLang(t *testing.T) {
	for _, c := range []struct {
		name  string
		lines []string
		want  string
	}{
		{"kana means japanese", []string{"Hello", "どうした?", "村川さん"}, "ja"},
		{"han alone is chinese", []string{"你好", "再见"}, "zh"},
		{"hangul", []string{"안녕하세요"}, "ko"},
		{"latin needs nothing", []string{"Hello there", "Stop it!"}, ""},
		{"empty", nil, ""},
	} {
		if got := translate.DetectLang(c.lines); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
