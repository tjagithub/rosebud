package subsrc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tjagithub/rosebud/internal/limit"
	"github.com/tjagithub/rosebud/internal/subs"
)

// Exec runs a program to completion and returns its stdout.
type Exec = subs.Exec

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
