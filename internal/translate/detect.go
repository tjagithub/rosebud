package translate

import "unicode"

// DetectLang guesses a language code from the scripts in lines, or "" when
// there is no Japanese, Chinese or Korean. Kana wins over Han.
func DetectLang(lines []string) string {
	var han, kana, hangul int
	for _, l := range lines {
		for _, r := range l {
			switch {
			case unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r):
				kana++
			case unicode.Is(unicode.Hangul, r):
				hangul++
			case unicode.Is(unicode.Han, r):
				han++
			}
		}
	}
	switch {
	case kana > 0:
		return "ja"
	case hangul > 0:
		return "ko"
	case han > 0:
		return "zh"
	}
	return ""
}
