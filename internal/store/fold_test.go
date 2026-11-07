package store_test

import (
	"testing"

	"github.com/tjagithub/rosebud/internal/store"
)

func TestFold(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"lower case", "Rosebud", "rosebud"},
		{"katakana to hiragana", "カタカナ", "かたかな"},
		{"small and voiced kana", "ッァヴヶ", "っぁゔゖ"},
		{"iteration marks", "ヽヾ", "ゝゞ"},
		{"long vowel mark stays", "ラーメン", "らーめん"},
		{"fullwidth ASCII", "ＲＯＳＥ１２３！", "rose123!"},
		{"ideographic space", "沈め　ろ", "沈め ろ"},
		{"kanji and hiragana stay", "村川さん", "村川さん"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := store.Fold(tt.in); got != tt.want {
				t.Errorf("Fold(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFold_Idempotent(t *testing.T) {
	for _, s := range []string{"ＲＯＳＥ", "カタカナ", "Mixed ケース　text", "村川さん!"} {
		once := store.Fold(s)
		if twice := store.Fold(once); twice != once {
			t.Errorf("Fold(Fold(%q)) = %q, want %q", s, twice, once)
		}
	}
}
