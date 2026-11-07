package store

import (
	"strings"
	"unicode"
)

// Fold maps text to its search form: lower case, fullwidth ASCII as ASCII,
// katakana as hiragana. Halfwidth katakana are left alone.
func Fold(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '　':
			return ' '
		case r >= '！' && r <= '～':
			return unicode.ToLower(r - 0xfee0)
		case r >= 'ァ' && r <= 'ヶ', r == 'ヽ', r == 'ヾ':
			return r - 0x60
		}
		return unicode.ToLower(r)
	}, s)
}
