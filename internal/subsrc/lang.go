package subsrc

import (
	"strings"

	"golang.org/x/text/language"
)

// bibliographic maps ISO 639-2/B codes, the older second spelling of about
// twenty languages, to their usual 639-2/T form.
var bibliographic = map[string]string{
	"alb": "sqi", "arm": "hye", "baq": "eus", "bur": "mya", "chi": "zho",
	"cze": "ces", "dut": "nld", "fre": "fra", "geo": "kat", "ger": "deu",
	"gre": "ell", "ice": "isl", "mac": "mkd", "mao": "mri", "may": "msa",
	"per": "fas", "rum": "ron", "slo": "slk", "tib": "bod", "wel": "cym",
}

// canon gives the three-letter form of an ISO 639-1 or 639-2 code. A code
// that is not a known language comes back lower-cased, matching only itself.
func canon(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if t, ok := bibliographic[code]; ok {
		code = t
	}
	base, err := language.ParseBase(code)
	if err != nil {
		return code
	}
	return base.ISO3()
}

func sameLang(a, b string) bool { return a != "" && b != "" && canon(a) == canon(b) }
