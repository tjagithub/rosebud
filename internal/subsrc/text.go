package subsrc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tjagithub/rosebud/internal/limit"
)

// Cue is one timed piece of a text subtitle track.
type Cue struct {
	Start, End time.Duration
	Text       string
}

// ReadText reads text track pos of file. ffmpeg turns any text codec (SubRip,
// ASS, WebVTT, mov_text) into SubRip, so one parser covers them all.
func ReadText(ctx context.Context, ex Exec, file string, pos int, timeout time.Duration) ([]Cue, error) {
	if file == "" {
		return nil, errors.New("subsrc: read text: empty file path")
	}
	out, err := limit.Run(ctx, "ffmpeg", timeout, func(ctx context.Context) ([]byte, error) {
		return ex(ctx, "ffmpeg", "-nostdin", "-v", "error", "-i", file, "-map", "0:s:"+strconv.Itoa(pos), "-f", "srt", "-")
	})
	if err != nil {
		return nil, fmt.Errorf("subsrc: read subtitle track %d of %s: %w", pos, file, err)
	}
	cues, err := ParseSRT(strings.NewReader(string(out)))
	if err != nil {
		return nil, fmt.Errorf("subsrc: subtitle track %d of %s: %w", pos, file, err)
	}
	return cues, nil
}

var (
	timing = regexp.MustCompile(`(\d+):(\d\d):(\d\d)[,.](\d{1,3})\s*-->\s*(\d+):(\d\d):(\d\d)[,.](\d{1,3})`)
	markup = regexp.MustCompile(`<[^>]*>|\{[^}]*\}`)
	bom    = string(rune(0xFEFF))
)

// ParseSRT reads SubRip, dropping markup such as <i> and {\an8} and cues left
// empty. Untimed blocks are skipped, but text with no timed block is an error.
func ParseSRT(r io.Reader) ([]Cue, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(strings.TrimPrefix(string(raw), bom), "\r\n", "\n")
	var cues []Cue
	blocks, timed := 0, 0
	for _, b := range strings.Split(text, "\n\n") {
		if strings.TrimSpace(b) == "" {
			continue
		}
		blocks++
		c, ok := parseBlock(b)
		if !ok {
			continue
		}
		timed++
		if c.Text != "" {
			cues = append(cues, c)
		}
	}
	if blocks > 0 && timed == 0 {
		return nil, errors.New("no SubRip cues found")
	}
	return cues, nil
}

// parseBlock reads one cue: an optional number, the timing line, then the text.
func parseBlock(b string) (Cue, bool) {
	lines := strings.Split(strings.TrimSpace(b), "\n")
	for i, l := range lines {
		m := timing.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		return Cue{Start: clock(m[1:5]), End: clock(m[5:9]), Text: cueText(lines[i+1:])}, true
	}
	return Cue{}, false
}

// clock turns hours, minutes, seconds and a fraction into a duration.
func clock(p []string) time.Duration {
	n := func(s string) time.Duration { v, _ := strconv.Atoi(s); return time.Duration(v) }
	frac := (p[3] + "00")[:3]
	return n(p[0])*time.Hour + n(p[1])*time.Minute + n(p[2])*time.Second + n(frac)*time.Millisecond
}

// cueText is the lines without markup or extra spaces, empty lines dropped.
func cueText(lines []string) string {
	var out []string
	for _, l := range lines {
		if l = strings.Join(strings.Fields(markup.ReplaceAllString(l, " ")), " "); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
