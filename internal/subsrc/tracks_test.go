package subsrc_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tjagithub/rosebud/internal/subsrc"
)

func TestParseTracks(t *testing.T) {
	for _, c := range []struct {
		name, in string
		want     []subsrc.Track
	}{
		{"empty", "", nil},
		{"positions count subtitle streams, not stream index", "2,subrip,eng,English SDH\n5,hdmv_pgs_subtitle,eng,Full\n", []subsrc.Track{
			{Pos: 0, Index: 2, Codec: "subrip", Lang: "eng", Title: "English SDH"},
			{Pos: 1, Index: 5, Codec: "hdmv_pgs_subtitle", Lang: "eng", Title: "Full"},
		}},
		{"missing tags", "3,dvd_subtitle\n4,dvd_subtitle,jpn\n", []subsrc.Track{
			{Pos: 0, Index: 3, Codec: "dvd_subtitle"},
			{Pos: 1, Index: 4, Codec: "dvd_subtitle", Lang: "jpn"},
		}},
		{"crlf and blank lines", "\r\n3,ass,jpn,Signs & Songs\r\n\r\n", []subsrc.Track{
			{Pos: 0, Index: 3, Codec: "ass", Lang: "jpn", Title: "Signs & Songs"},
		}},
		{"quoted title with a comma", "3,subrip,eng,\"Dialogue, full\"\n", []subsrc.Track{
			{Pos: 0, Index: 3, Codec: "subrip", Lang: "eng", Title: "Dialogue, full"},
		}},
		{"junk lines skipped without taking a position", "x,y\nnot a track\n7,subrip,jpn\n", []subsrc.Track{
			{Pos: 0, Index: 7, Codec: "subrip", Lang: "jpn"},
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := subsrc.ParseTracks(strings.NewReader(c.in))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestKind(t *testing.T) {
	for codec, want := range map[string]subsrc.Kind{
		"subrip": subsrc.Text, "ass": subsrc.Text, "ssa": subsrc.Text, "webvtt": subsrc.Text,
		"mov_text": subsrc.Text, "text": subsrc.Text,
		"dvd_subtitle": subsrc.Bitmap, "hdmv_pgs_subtitle": subsrc.Bitmap,
		"dvb_subtitle": subsrc.Bitmap, "xsub": subsrc.Bitmap,
		"": subsrc.Unknown, "eia_608": subsrc.Unknown,
	} {
		if got := (subsrc.Track{Codec: codec}).Kind(); got != want {
			t.Errorf("Kind(%q) = %v, want %v", codec, got, want)
		}
	}
}

func TestPick(t *testing.T) {
	tr := func(pos int, codec, lang, title string) subsrc.Track {
		return subsrc.Track{Pos: pos, Index: pos + 2, Codec: codec, Lang: lang, Title: title}
	}
	for _, c := range []struct {
		name   string
		tracks []subsrc.Track
		lang   string
		want   int // position, -1 for none
	}{
		{"no tracks", nil, "jpn", -1},
		{"no track in the language", []subsrc.Track{tr(0, "subrip", "eng", "")}, "jpn", -1},
		{"language match ignores case", []subsrc.Track{tr(0, "dvd_subtitle", "JPN", "")}, "jpn", 0},
		{"picks the right language", []subsrc.Track{tr(0, "dvd_subtitle", "eng", ""), tr(1, "dvd_subtitle", "jpn", "")}, "jpn", 1},
		{"text beats bitmap", []subsrc.Track{tr(0, "dvd_subtitle", "jpn", ""), tr(1, "subrip", "jpn", "")}, "jpn", 1},
		{"full beats forced", []subsrc.Track{tr(0, "subrip", "jpn", "Forced"), tr(1, "subrip", "jpn", "Full")}, "jpn", 1},
		{"full beats signs and songs", []subsrc.Track{tr(0, "ass", "jpn", "Signs & Songs"), tr(1, "ass", "jpn", "Full")}, "jpn", 1},
		{"forced text still beats nothing", []subsrc.Track{tr(0, "subrip", "jpn", "Forced")}, "jpn", 0},
		{"first of equals", []subsrc.Track{tr(0, "dvd_subtitle", "jpn", ""), tr(1, "dvd_subtitle", "jpn", "")}, "jpn", 0},
		{"unreadable codec never picked", []subsrc.Track{tr(0, "eia_608", "jpn", "")}, "jpn", -1},
		{"readable beats unreadable", []subsrc.Track{tr(0, "eia_608", "jpn", ""), tr(1, "dvd_subtitle", "jpn", "")}, "jpn", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, ok := subsrc.Pick(c.tracks, c.lang)
			switch {
			case c.want < 0 && ok:
				t.Errorf("picked %+v, want none", got)
			case c.want >= 0 && (!ok || got.Pos != c.want):
				t.Errorf("got %+v ok=%v, want position %d", got, ok, c.want)
			}
		})
	}
}

func TestPickEmptyLanguageFindsNothing(t *testing.T) {
	tracks := []subsrc.Track{{Codec: "dvd_subtitle"}, {Codec: "subrip", Lang: "jpn"}}
	if got, ok := subsrc.Pick(tracks, ""); ok {
		t.Errorf("picked %+v for an empty language", got)
	}
}

func TestPickAcceptsTwoAndThreeLetterCodes(t *testing.T) {
	jpn := []subsrc.Track{{Pos: 0, Index: 2, Codec: "dvd_subtitle", Lang: "jpn"}}
	for _, l := range []string{"ja", "JA", "jpn"} {
		if _, ok := subsrc.Pick(jpn, l); !ok {
			t.Errorf("Pick(jpn track, %q) found nothing", l)
		}
	}
	if _, ok := subsrc.Pick(jpn, "en"); ok {
		t.Error("en matched a jpn track")
	}
	fre := []subsrc.Track{{Codec: "subrip", Lang: "fre"}, {Pos: 1, Codec: "subrip", Lang: "fra"}}
	for _, l := range []string{"fr", "fra", "fre"} {
		if _, ok := subsrc.Pick(fre, l); !ok {
			t.Errorf("French code %q found nothing", l)
		}
	}
	if _, ok := subsrc.Pick([]subsrc.Track{{Codec: "subrip", Lang: "ja"}}, "jpn"); !ok {
		t.Error("a two-letter track tag did not match jpn")
	}
}

func TestPickLanguageCodes(t *testing.T) {
	found := func(trackLang, want string) bool {
		_, ok := subsrc.Pick([]subsrc.Track{{Codec: "subrip", Lang: trackLang}}, want)
		return ok
	}
	for _, c := range [][2]string{
		{"tur", "tr"},
		{"tr", "tur"},
		{"kor", "ko"},
		{"ara", "ar"},
		{"hin", "hi"},
		{"fre", "fr"},
		{"fra", "fre"},
		{"ger", "de"},
		{"chi", "zh"},
		{"zho", "chi"},
		{"cze", "cs"},
		{"gre", "el"},
		{"wel", "cy"},
		{"per", "fa"},
		{"rum", "ro"},
		{"alb", "sqi"},
		{"arm", "hye"},
		{"baq", "eus"},
		{"bur", "mya"},
		{"dut", "nl"},
		{"geo", "kat"},
		{"ice", "isl"},
		{"mac", "mkd"},
		{"mao", "mri"},
		{"may", "msa"},
		{"slo", "slk"},
		{"tib", "bod"},
		{"xx", "xx"},
	} {
		if !found(c[0], c[1]) {
			t.Errorf("track %q was not found by %q", c[0], c[1])
		}
	}
	for _, c := range [][2]string{{"jpn", "kor"}, {"fre", "ger"}, {"xx", "yy"}, {"jpn", ""}, {"", "jpn"}, {"jpn", "jp"}} {
		if found(c[0], c[1]) {
			t.Errorf("track %q was found by %q", c[0], c[1])
		}
	}
}
