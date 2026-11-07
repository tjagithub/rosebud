package subsrc

import "strings"

// iso3 maps two-letter language codes to the three-letter ones that files tag
// tracks with. Codes not listed only match themselves.
var iso3 = map[string]string{
	"en": "eng", "ja": "jpn", "fr": "fra", "de": "deu", "es": "spa", "it": "ita",
	"pt": "por", "ru": "rus", "ko": "kor", "zh": "zho", "nl": "nld", "sv": "swe",
}

// alt maps older bibliographic three-letter codes to the usual ones.
var alt = map[string]string{"fre": "fra", "ger": "deu", "chi": "zho", "dut": "nld"}

func canon(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if c, ok := iso3[code]; ok {
		return c
	}
	if c, ok := alt[code]; ok {
		return c
	}
	return code
}

func sameLang(a, b string) bool { return a != "" && b != "" && canon(a) == canon(b) }
