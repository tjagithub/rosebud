// Package subsrc finds a film's subtitle tracks and says how to read each one:
// text tracks can be parsed directly, bitmap tracks need OCR.
package subsrc

import (
	"encoding/csv"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
)

// Track is one subtitle stream. Pos counts subtitle streams only, so it is
// the N in ffmpeg's "0:s:N"; Index is the stream's number in the whole file.
// Path is set for a subtitle file beside the video, whose only track it is.
type Track struct {
	Pos, Index         int
	Codec, Lang, Title string
	Path               string
}

// Kind says how a track's events can be read.
type Kind int

const (
	Unknown Kind = iota // no way to read it
	Text                // parse the text directly
	Bitmap              // draw it and OCR it
)

// Kind classifies the track by its codec.
func (t Track) Kind() Kind {
	switch t.Codec {
	case "subrip", "srt", "ass", "ssa", "webvtt", "mov_text", "text":
		return Text
	case "dvd_subtitle", "hdmv_pgs_subtitle", "dvb_subtitle", "xsub":
		return Bitmap
	}
	return Unknown
}

// ParseTracks reads ffprobe's "index,codec_name,language,title" csv lines.
// Lines that are not a track are skipped.
func ParseTracks(r io.Reader) ([]Track, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	var out []Track
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if len(rec) < 2 {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimSpace(rec[0]))
		if err != nil || strings.TrimSpace(rec[1]) == "" {
			continue
		}
		t := Track{Pos: len(out), Index: idx, Codec: strings.TrimSpace(rec[1])}
		if len(rec) > 2 {
			t.Lang = strings.TrimSpace(rec[2])
		}
		if len(rec) > 3 {
			t.Title = strings.TrimSpace(rec[3])
		}
		out = append(out, t)
	}
}

// Pick chooses the best readable track in lang: a full track over forced or
// signs-only ones, text over bitmap, a file beside the video, then the earliest.
func Pick(tracks []Track, lang string) (Track, bool) {
	if lang == "" {
		return Track{}, false
	}
	return best(tracks, func(t Track) bool { return sameLang(t.Lang, lang) }, func(Track) int { return 0 })
}

func partial(title string) bool {
	title = strings.ToLower(title)
	return strings.Contains(title, "forced") || strings.Contains(title, "signs")
}

// PickAuto chooses a track when none is asked for: the spoken language, then
// English, then any other; within a language as Pick does. spoken may be empty.
func PickAuto(tracks []Track, spoken string) (Track, bool) {
	lang := func(t Track) int {
		switch {
		case sameLang(t.Lang, spoken):
			return 2
		case sameLang(t.Lang, "en"):
			return 1
		}
		return 0
	}
	return best(tracks, func(Track) bool { return true }, lang)
}

// best is the earliest readable track that ok accepts with the highest
// rank: first by lang, then full, text, and beside the video.
func best(tracks []Track, ok func(Track) bool, lang func(Track) int) (Track, bool) {
	rank := func(t Track) [4]int {
		return [4]int{lang(t), b2i(!partial(t.Title)), b2i(t.Kind() == Text), b2i(t.Path != "")}
	}
	var out Track
	var top [4]int
	found := false
	for _, t := range tracks {
		if t.Kind() == Unknown || !ok(t) {
			continue
		}
		if r := rank(t); !found || slices.Compare(r[:], top[:]) > 0 {
			out, top, found = t, r, true
		}
	}
	return out, found
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
