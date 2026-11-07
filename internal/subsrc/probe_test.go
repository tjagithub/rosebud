package subsrc_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/subsrc"
)

func probeWith(out string, err error, got *[]string) subsrc.Exec {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		*got = append([]string{name}, args...)
		return []byte(out), err
	}
}

func TestProbe(t *testing.T) {
	var got []string
	tracks, err := subsrc.Probe(t.Context(), probeWith("2,dvd_subtitle,eng\n3,dvd_subtitle,jpn\n", nil, &got), "/films/-odd name.mkv", testTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 || tracks[1].Lang != "jpn" || tracks[1].Pos != 1 {
		t.Errorf("tracks = %+v", tracks)
	}
	i := slices.Index(got, "-i")
	if got[0] != "ffprobe" || i < 0 || got[i+1] != "/films/-odd name.mkv" {
		t.Errorf("command = %q, want the path as the value of -i", got)
	}
}

func TestProbeErrors(t *testing.T) {
	var got []string
	boom := errors.New("boom")
	if _, err := subsrc.Probe(t.Context(), probeWith("", boom, &got), "f.mkv", testTimeout); !errors.Is(err, boom) {
		t.Errorf("exec error = %v", err)
	}
	got = nil
	if _, err := subsrc.Probe(t.Context(), probeWith("", nil, &got), "", testTimeout); err == nil {
		t.Error("empty path: no error")
	}
	if len(got) != 0 {
		t.Errorf("ran %q for an empty path", got)
	}
}

func TestProbeTimesOutOnAHang(t *testing.T) {
	hang := func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	_, err := subsrc.Probe(t.Context(), hang, "f.mkv", 50*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "ffprobe took over 50ms to complete") {
		t.Errorf("err = %v", err)
	}
}

func TestProbeNeedsATimeout(t *testing.T) {
	var got []string
	if _, err := subsrc.Probe(t.Context(), probeWith("", nil, &got), "f.mkv", 0); err == nil {
		t.Error("no error for a zero timeout")
	}
	if len(got) != 0 {
		t.Errorf("ran %q with no usable timeout", got)
	}
}

const testTimeout = time.Minute
