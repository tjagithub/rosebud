package scenes_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/scenes"
)

func TestParseHWAccels(t *testing.T) {
	for name, c := range map[string]struct {
		in   string
		want []string
	}{
		"mac":       {"Hardware acceleration methods:\nvideotoolbox\n", []string{"videotoolbox"}},
		"several":   {"Hardware acceleration methods:\ncuda\nvaapi\nvulkan\n\n", []string{"cuda", "vaapi", "vulkan"}},
		"crlf":      {"Hardware acceleration methods:\r\nqsv\r\n", []string{"qsv"}},
		"none":      {"Hardware acceleration methods:\n", nil},
		"empty":     {"", nil},
		"duplicate": {"Hardware acceleration methods:\ncuda\ncuda\n", []string{"cuda"}},
		"noise":     {"ffmpeg version x\nHardware acceleration methods:\ncuda\nsome warning: here\n", []string{"cuda"}},
	} {
		if got := scenes.ParseHWAccels(c.in); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

func TestHWAccels(t *testing.T) {
	var got []string
	ex := func(_ context.Context, name string, args ...string) ([]byte, error) {
		got = append([]string{name}, args...)
		return []byte("Hardware acceleration methods:\nvideotoolbox\n"), nil
	}
	list, err := scenes.HWAccels(t.Context(), ex, time.Minute)
	if err != nil || !slices.Equal(list, []string{"videotoolbox"}) || !slices.Contains(got, "-hwaccels") {
		t.Errorf("list %q err %v cmd %q", list, err, got)
	}
	boom := errors.New("boom")
	if _, err := scenes.HWAccels(t.Context(), func(context.Context, string, ...string) ([]byte, error) { return nil, boom }, time.Minute); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
	if _, err := scenes.HWAccels(t.Context(), ex, 0); err == nil {
		t.Error("zero timeout: no error")
	}
}

func TestCheckHWAccel(t *testing.T) {
	avail := []string{"cuda", "vaapi"}
	for method, ok := range map[string]bool{"": true, "auto": true, "cuda": true, "vaapi": true, "videotoolbox": false} {
		if err := scenes.CheckHWAccel(method, avail); (err == nil) != ok {
			t.Errorf("CheckHWAccel(%q) = %v, want ok=%t", method, err, ok)
		}
	}
	if err := scenes.CheckHWAccel("qsv", nil); err == nil {
		t.Error("nothing available: no error")
	}
}
