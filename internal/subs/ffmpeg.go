package subs

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
// named by their millisecond bounds.
//
// A video that starts late must not make a seek drop the audio before it.
// For mov-family files -ignore_editlist fixes that. Other demuxers reject the
// option and seek by the video's keyframes, so a span that starts before the
// video does is seeked on the output side, which is exact but decodes from the
// start; later spans seek on the input.
func FFmpegCutter(ex Exec, file, dir string) Cutter {
	var once sync.Once
	var lead time.Duration
	return func(ctx context.Context, s Span) (string, error) {
		wav := filepath.Join(dir, fmt.Sprintf("%08d-%08d.wav", s.Start.Milliseconds(), s.End.Milliseconds()))
		args := []string{"-nostdin", "-y", "-v", "error"}
		seek, length := []string{"-ss", secs(s.Start)}, []string{"-t", secs(s.End - s.Start)}
		switch {
		case isMov(file):
			args = append(args, seek...)
			args = append(args, length...)
			args = append(args, "-ignore_editlist", "1", "-i", file)
		default:
			once.Do(func() { lead = videoStart(ctx, ex, file) })
			if s.Start >= lead {
				args = append(append(args, seek...), length...)
				args = append(args, "-i", file)
			} else {
				args = append(args, "-i", file)
				args = append(append(args, seek...), length...)
			}
		}
		args = append(args, "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wav)
		_, err := ex(ctx, "ffmpeg", args...)
		if err != nil {
			return "", fmt.Errorf("subs: cut %s: %w", wav, err)
		}
		return wav, nil
	}
}

// videoStart is when file's first video frame shows. If it cannot be told it
// is as late as possible, so every cut seeks the exact way.
func videoStart(ctx context.Context, ex Exec, file string) time.Duration {
	out, err := ex(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=start_time", "-of", "csv=p=0", file)
	if err != nil {
		return math.MaxInt64
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || f < 0 {
		return math.MaxInt64
	}
	return time.Duration(f * float64(time.Second))
}

// isMov reports whether file's name says the mov demuxer reads it.
func isMov(file string) bool {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".mp4", ".m4v", ".m4a", ".mov", ".3gp", ".3g2":
		return true
	}
	return false
}

func secs(d time.Duration) string { return strconv.FormatFloat(d.Seconds(), 'f', -1, 64) }
