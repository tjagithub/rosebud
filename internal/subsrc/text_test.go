package subsrc_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/subsrc"
)

const bom = "\xef\xbb\xbf"

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func TestParseSRT(t *testing.T) {
	in := bom + "1\r\n00:00:01,000 --> 00:00:02,500\r\nHello\r\nthere\r\n\r\n" +
		"2\n00:01:02.250 --> 00:01:04,000 X1:10 X2:20\n<i>In italics</i> {\\an8}up top\n\n\n" +
		"3\n00:02:00,000 --> 00:02:01,000\n<font color=\"red\"></font>\n\n" +
		"4\n01:00:00,000 --> 01:00:01,000\nLast"
	got, err := subsrc.ParseSRT(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []subsrc.Cue{
		{Start: ms(1000), End: ms(2500), Text: "Hello\nthere"},
		{Start: ms(62250), End: ms(64000), Text: "In italics up top"},
		{Start: time.Hour, End: time.Hour + time.Second, Text: "Last"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestParseSRTSkipsBrokenBlocksButNotEverything(t *testing.T) {
	got, err := subsrc.ParseSRT(strings.NewReader("1\nnot a time\nhi\n\n2\n00:00:01,000 --> 00:00:02,000\nok\n"))
	if err != nil || len(got) != 1 || got[0].Text != "ok" {
		t.Errorf("got %+v, %v; want the one good cue", got, err)
	}
	if _, err := subsrc.ParseSRT(strings.NewReader("garbage\nmore garbage\n")); err == nil {
		t.Error("text with no cue at all must be an error")
	}
	if got, err := subsrc.ParseSRT(strings.NewReader("")); err != nil || len(got) != 0 {
		t.Errorf("an empty track is no cues and no error, got %+v, %v", got, err)
	}
}

func TestReadTextAsksFFmpegForSubRipOfTheTrack(t *testing.T) {
	var args []string
	ex := func(_ context.Context, name string, a ...string) ([]byte, error) {
		args = append([]string{name}, a...)
		return []byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"), nil
	}
	cues, err := subsrc.ReadText(t.Context(), ex, "f.mkv", 2, time.Second)
	if err != nil || len(cues) != 1 {
		t.Fatalf("got %+v, %v", cues, err)
	}
	joined := strings.Join(args, " ")
	if !strings.HasPrefix(joined, "ffmpeg -nostdin") || !strings.Contains(joined, "-map 0:s:2") || !strings.HasSuffix(joined, "-f srt -") {
		t.Errorf("ffmpeg args: %s", joined)
	}
}

func TestReadTextFailsOnFFmpegErrorAndEmptyPath(t *testing.T) {
	ex := func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("boom") }
	if _, err := subsrc.ReadText(t.Context(), ex, "f.mkv", 0, time.Second); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("want the ffmpeg error, got %v", err)
	}
	if _, err := subsrc.ReadText(t.Context(), ex, "", 0, time.Second); err == nil {
		t.Error("an empty path must be refused")
	}
}

func TestAudioLangReadsTheFirstAudioTrackTag(t *testing.T) {
	for out, want := range map[string]string{"jpn\n": "jpn", "und\n": "", "\n": "", "eng\nfra\n": "eng", "garbage!\n": ""} {
		var args string
		ex := func(_ context.Context, name string, a ...string) ([]byte, error) {
			args = name + " " + strings.Join(a, " ")
			return []byte(out), nil
		}
		if got := subsrc.AudioLang(t.Context(), ex, "f.mkv", time.Second); got != want {
			t.Errorf("ffprobe said %q: got %q, want %q", out, got, want)
		}
		if !strings.Contains(args, "-select_streams a:0") {
			t.Errorf("args: %s", args)
		}
	}
	fail := func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("boom") }
	if got := subsrc.AudioLang(t.Context(), fail, "f.mkv", time.Second); got != "" {
		t.Errorf("a failed probe is unknown, got %q", got)
	}
}

// TestReadTextRealFFmpeg muxes SubRip and ASS tracks into a tiny mkv and reads both back.
func TestReadTextRealFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	mkv := textFixture(t)
	run := func(ctx context.Context, name string, a ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, a...).Output()
	}
	tracks, err := subsrc.Probe(t.Context(), run, mkv, 10*time.Second)
	if err != nil || len(tracks) != 2 || tracks[0].Kind() != subsrc.Text || tracks[1].Kind() != subsrc.Text {
		t.Fatalf("tracks %+v, %v", tracks, err)
	}
	for _, tr := range tracks {
		cues, err := subsrc.ReadText(t.Context(), run, mkv, tr.Pos, 10*time.Second)
		if err != nil || len(cues) != 2 || cues[1].Start != ms(2500) || !strings.Contains(cues[1].Text, "second") {
			t.Errorf("track %d (%s): %+v, %v", tr.Pos, tr.Codec, cues, err)
		}
	}
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// textFixture is a 4 s mkv with an English SubRip track and a Japanese ASS track.
func textFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	srt := filepath.Join(dir, "en.srt")
	ass := filepath.Join(dir, "ja.ass")
	write(t, srt, "1\n00:00:00,500 --> 00:00:01,500\nfirst line\n\n2\n00:00:02,500 --> 00:00:03,500\n<i>second</i> line\n")
	write(t, ass, "[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize\nStyle: Default,Arial,20\n\n"+
		"[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"+
		"Dialogue: 0,0:00:00.50,0:00:01.50,Default,,0,0,0,,最初\n"+
		"Dialogue: 0,0:00:02.50,0:00:03.50,Default,,0,0,0,,{\\i1}second{\\i0} 二番目\n")
	mkv := filepath.Join(dir, "f.mkv")
	cmd := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "color=black:s=64x36:d=4",
		"-i", srt, "-i", ass, "-map", "0:v", "-map", "1:s", "-map", "2:s", "-c:s:0", "srt", "-c:s:1", "ass",
		"-metadata:s:s:0", "language=eng", "-metadata:s:s:1", "language=jpn", mkv)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make fixture: %v\n%s", err, out)
	}
	return mkv
}
