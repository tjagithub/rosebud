// Package subsrc finds a film's subtitle tracks and says how to read each one:
// text tracks can be parsed directly, bitmap tracks need OCR.
package subsrc

import (
	"encoding/csv"
	"errors"
	"io"
	"strconv"
	"strings"
)

// Track is one subtitle stream. Pos counts subtitle streams only, so it is
// the N in ffmpeg's "0:s:N"; Index is the stream's number in the whole file.
type Track struct {
	Pos, Index         int
	Codec, Lang, Title string
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

// Pick chooses the best readable track in lang: text over bitmap, a full
// track over forced or signs-only ones, then the earliest.
func Pick(tracks []Track, lang string) (Track, bool) {
	if lang == "" {
		return Track{}, false
	}
	var best Track
	bestScore := 0
	for _, t := range tracks {
		if !strings.EqualFold(t.Lang, lang) || t.Kind() == Unknown {
			continue
		}
		s := 1
		if t.Kind() == Text {
			s += 2
		}
		if !partial(t.Title) {
			s += 4
		}
		if s > bestScore {
			best, bestScore = t, s
		}
	}
	return best, bestScore > 0
}

func partial(title string) bool {
	title = strings.ToLower(title)
	return strings.Contains(title, "forced") || strings.Contains(title, "signs")
}
