package subsrc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tjagithub/rosebud/internal/limit"
	"github.com/tjagithub/rosebud/internal/subs"
)

// Exec runs a program to completion and returns its stdout.
type Exec = subs.Exec

// AudioLang is the language tag of file's first audio track, or "" when it has
// none, it is "und", or the probe fails: the tag is a hint, never needed.
func AudioLang(ctx context.Context, ex Exec, file string, timeout time.Duration) string {
	out, err := limit.Run(ctx, "ffprobe", timeout, func(ctx context.Context) ([]byte, error) {
		return ex(ctx, "ffprobe", "-v", "error", "-select_streams", "a:0",
			"-show_entries", "stream_tags=language", "-of", "default=nw=1:nk=1", "-i", file)
	})
	if err != nil {
		return ""
	}
	tag, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if tag == "und" || !isLang(tag) {
		return ""
	}
	return canon(tag)
}

// Probe lists the subtitle tracks of file with ffprobe. It gives up after
// timeout, which must be positive, so a stuck probe cannot hang the caller.
func Probe(ctx context.Context, ex Exec, file string, timeout time.Duration) ([]Track, error) {
	if file == "" {
		return nil, errors.New("subsrc: probe: empty file path")
	}
	out, err := limit.Run(ctx, "ffprobe", timeout, func(ctx context.Context) ([]byte, error) {
		return ex(ctx, "ffprobe", "-v", "error", "-select_streams", "s",
			"-show_entries", "stream=index,codec_name:stream_tags=language,title",
			"-of", "csv=p=0", "-i", file)
	})
	if err != nil {
		return nil, fmt.Errorf("subsrc: probe %s: %w", file, err)
	}
	return ParseTracks(bytes.NewReader(out))
}
