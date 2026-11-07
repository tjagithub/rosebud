package subs

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Exec runs a program to completion and returns its stdout.
type Exec func(ctx context.Context, name string, args ...string) ([]byte, error)

// Duration is the length of a media file, from ffprobe.
func Duration(ctx context.Context, ex Exec, file string) (time.Duration, error) {
	out, err := ex(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", file)
	if err != nil {
		return 0, fmt.Errorf("subs: probe %s: %w", file, err)
	}
	d, ok := seconds(strings.TrimSpace(string(out)))
	if !ok || d <= 0 {
		return 0, fmt.Errorf("subs: probe %s: bad duration %q", file, bytes.TrimSpace(out))
	}
	return d, nil
}

// DetectSilences finds the quiet stretches of a file's audio.
func DetectSilences(ctx context.Context, ex Exec, file string, total time.Duration) ([]Span, error) {
	out, err := ex(ctx, "ffmpeg", "-nostdin", "-v", "error", "-i", file, "-vn",
		"-af", "silencedetect=noise=-30dB:d=0.5,ametadata=mode=print:file=-", "-f", "null", "-")
	if err != nil {
		return nil, fmt.Errorf("subs: silence in %s: %w", file, err)
	}
	return ParseSilences(bytes.NewReader(out), total)
}

// FFmpegCutter cuts spans of file's audio into 16 kHz mono WAVs in dir,
// named by their millisecond bounds. -ignore_editlist keeps a late-starting
// video stream from making the seek drop audio that comes before it.
func FFmpegCutter(ex Exec, file, dir string) Cutter {
	return func(ctx context.Context, s Span) (string, error) {
		wav := filepath.Join(dir, fmt.Sprintf("%08d-%08d.wav", s.Start.Milliseconds(), s.End.Milliseconds()))
		_, err := ex(ctx, "ffmpeg", "-nostdin", "-y", "-v", "error",
			"-ss", secs(s.Start), "-t", secs(s.End-s.Start), "-ignore_editlist", "1", "-i", file,
			"-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wav)
		if err != nil {
			return "", fmt.Errorf("subs: cut %s: %w", wav, err)
		}
		return wav, nil
	}
}

func secs(d time.Duration) string { return strconv.FormatFloat(d.Seconds(), 'f', -1, 64) }
