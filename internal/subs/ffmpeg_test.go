package subs_test

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/scenes"
	"github.com/tjagithub/rosebud/internal/subs"
)

type call struct {
	name string
	args []string
}

func fakeExec(out string, err error, calls *[]call) subs.Exec {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		*calls = append(*calls, call{name, args})
		return []byte(out), err
	}
}

func has(args []string, seq ...string) bool {
	for i := range args {
		if strings.HasPrefix(strings.Join(args[i:], "\x00"), strings.Join(seq, "\x00")) {
			return true
		}
	}
	return false
}

func TestDuration(t *testing.T) {
	tests := []struct {
		name, out string
		want      time.Duration
		bad       bool
	}{
		{"seconds", "5400.512\n", 5400*time.Second + 512*time.Millisecond, false},
		{"padded", "  60.256 \r\n", 60*time.Second + 256*time.Millisecond, false},
		{"empty", "", 0, true},
		{"N/A", "N/A\n", 0, true},
		{"negative", "-3\n", 0, true},
		{"NaN", "NaN\n", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []call
			got, err := subs.Duration(t.Context(), fakeExec(tt.out, nil, &calls), "film.mkv")
			if (err != nil) != tt.bad || got != tt.want {
				t.Errorf("Duration = %v, %v; want %v, bad=%v", got, err, tt.want, tt.bad)
			}
			if calls[0].name != "ffprobe" || !slices.Contains(calls[0].args, "film.mkv") {
				t.Errorf("call = %v", calls[0])
			}
		})
	}
}

func TestDuration_ExecError(t *testing.T) {
	boom := errors.New("boom")
	var calls []call
	if _, err := subs.Duration(t.Context(), fakeExec("", boom, &calls), "f.mkv"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestDetectSilences(t *testing.T) {
	var calls []call
	out := "lavfi.silence_start=1\nlavfi.silence_end=2.5\n"
	got, err := subs.DetectSilences(t.Context(), fakeExec(out, nil, &calls), "f.mkv", sec(10))
	if err != nil {
		t.Fatal(err)
	}
	if want := []subs.Span{{sec(1), sec(2) + 500*time.Millisecond}}; !slices.Equal(got, want) {
		t.Errorf("spans = %v, want %v", got, want)
	}
	a := calls[0].args
	if calls[0].name != "ffmpeg" || !has(a, "-i", "f.mkv") || !has(a, "-vn") || !has(a, "-nostdin") {
		t.Errorf("call = %v", calls[0])
	}
	af := a[slices.Index(a, "-af")+1]
	if !strings.Contains(af, "silencedetect=noise=-30dB:d=0.5") || !strings.Contains(af, "ametadata=mode=print:file=-") {
		t.Errorf("-af = %q", af)
	}
}

func TestDetectSilences_ExecError(t *testing.T) {
	boom := errors.New("boom")
	var calls []call
	if _, err := subs.DetectSilences(t.Context(), fakeExec("", boom, &calls), "f.mkv", sec(10)); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestFFmpegCutter(t *testing.T) {
	dir := t.TempDir()
	var calls []call
	cut := subs.FFmpegCutter(fakeExec("0", nil, &calls), "f.mkv", dir)

	wav, err := cut(t.Context(), subs.Span{Start: 90*time.Second + 500*time.Millisecond, End: 120 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "00090500-00120000.wav"); wav != want {
		t.Errorf("wav = %q, want %q", wav, want)
	}
	a := calls[len(calls)-1].args
	for _, seq := range [][]string{
		{"-ss", "90.5"}, {"-t", "29.5"}, {"-i", "f.mkv"}, {"-vn"}, {"-ac", "1"}, {"-ar", "16000"}, {"-nostdin"}, {"-y"},
	} {
		if !has(a, seq...) {
			t.Errorf("args %v lack %v", a, seq)
		}
	}
	if a[len(a)-1] != wav {
		t.Errorf("last arg = %q, want output %q", a[len(a)-1], wav)
	}
	// -ss before -i seeks the input, so the cut is exact on decoded audio.
	if slices.Index(a, "-ss") > slices.Index(a, "-i") {
		t.Errorf("-ss after -i: %v", a)
	}
}

// -ignore_editlist belongs to the mov demuxer: ffmpeg refuses it for any other
// container, and it must come before -i to act.
func TestFFmpegCutter_IgnoreEditlistOnlyForMov(t *testing.T) {
	for file, want := range map[string]bool{
		"f.mkv": false, "f.avi": false, "f.webm": false,
		"f.mp4": true, "F.MOV": true, "f.m4v": true, "f.m4a": true,
	} {
		var calls []call
		cut := subs.FFmpegCutter(fakeExec("", nil, &calls), file, t.TempDir())
		if _, err := cut(t.Context(), subs.Span{End: sec(1)}); err != nil {
			t.Fatal(err)
		}
		a := calls[len(calls)-1].args
		i := slices.Index(a, "-ignore_editlist")
		if (i >= 0) != want {
			t.Errorf("%s: -ignore_editlist present = %v, want %v: %v", file, i >= 0, want, a)
		}
		if want && i > slices.Index(a, "-i") {
			t.Errorf("%s: -ignore_editlist after -i: %v", file, a)
		}
	}
}

func TestFFmpegCutter_MatroskaSeeksOutputSideBeforeVideoStarts(t *testing.T) {
	var calls []call
	cut := subs.FFmpegCutter(fakeExec("3.408000\n", nil, &calls), "f.mkv", t.TempDir())
	for _, start := range []time.Duration{time.Second, 10 * time.Second} {
		if _, err := cut(t.Context(), subs.Span{Start: start, End: start + 5*time.Second}); err != nil {
			t.Fatal(err)
		}
	}
	var cuts [][]string
	for _, c := range calls {
		if c.name == "ffmpeg" {
			cuts = append(cuts, c.args)
		}
	}
	if len(cuts) != 2 || slices.Index(cuts[0], "-ss") < slices.Index(cuts[0], "-i") {
		t.Errorf("early span must seek after -i: %v", cuts)
	}
	if slices.Index(cuts[1], "-ss") > slices.Index(cuts[1], "-i") {
		t.Errorf("later span must seek before -i: %v", cuts[1])
	}
}

// The real ffmpeg must accept the cut of a matroska file.
func TestFFmpegCutter_Matroska(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	file := filepath.Join(t.TempDir(), "a.mkv")
	out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-itsoffset", "3", "-f", "lavfi", "-i", "color=c=red:s=64x64:r=10:d=8",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=10",
		"-map", "0:v", "-map", "1:a", "-pix_fmt", "yuv420p", file).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
	ex := subs.Exec(scenes.NewExec())
	cut := subs.FFmpegCutter(ex, file, t.TempDir())
	for _, start := range []time.Duration{0, time.Second, 2 * time.Second, 4 * time.Second} {
		wav, err := cut(t.Context(), subs.Span{Start: start, End: start + 5*time.Second})
		if err != nil {
			t.Fatal(err)
		}
		got, err := subs.Duration(t.Context(), ex, wav)
		if err != nil {
			t.Fatal(err)
		}
		if d := got - 5*time.Second; d < -50*time.Millisecond || d > 50*time.Millisecond {
			t.Errorf("cut from %v lasts %v, want 5s", start, got)
		}
	}
}

// A film whose video starts late must still give all the audio from 0.
func TestFFmpegCutter_LateVideoStart(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	file := filepath.Join(t.TempDir(), "late.mp4")
	out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-itsoffset", "3", "-f", "lavfi", "-i", "color=c=red:s=64x64:r=10:d=8",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=10",
		"-map", "0:v", "-map", "1:a", "-pix_fmt", "yuv420p", file).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
	ex := subs.Exec(scenes.NewExec())
	cut := subs.FFmpegCutter(ex, file, t.TempDir())
	for _, start := range []time.Duration{0, time.Second, 2 * time.Second} {
		wav, err := cut(t.Context(), subs.Span{Start: start, End: start + 5*time.Second})
		if err != nil {
			t.Fatal(err)
		}
		got, err := subs.Duration(t.Context(), ex, wav)
		if err != nil {
			t.Fatal(err)
		}
		if d := got - 5*time.Second; d < -50*time.Millisecond || d > 50*time.Millisecond {
			t.Errorf("cut from %v lasts %v, want 5s", start, got)
		}
	}
}

func TestFFmpegCutter_ExecError(t *testing.T) {
	boom := errors.New("boom")
	var calls []call
	cut := subs.FFmpegCutter(fakeExec("", boom, &calls), "f.mkv", t.TempDir())
	if _, err := cut(t.Context(), subs.Span{End: sec(1)}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}
