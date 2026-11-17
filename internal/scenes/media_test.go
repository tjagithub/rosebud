package scenes_test

import (
	"math"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/scenes"
)

func TestParseMedia(t *testing.T) {
	out := `{"format":{"duration":"5400.250000"},"streams":[{"avg_frame_rate":"30000/1001"}],"chapters":[
		{"start_time":"0.000000","tags":{"title":"Intro"}},
		{"start_time":"2.500000","tags":{}},
		{"start_time":"bad"}]}`
	m, err := scenes.ParseMedia([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	fps, ch := m.FPS, m.Chapters
	if math.Abs(fps-29.97) > 0.01 {
		t.Errorf("fps = %v", fps)
	}
	if m.Duration != 5400250*time.Millisecond {
		t.Errorf("duration = %v", m.Duration)
	}
	if len(ch) != 2 || ch[0].Title != "Intro" || ch[1].Start != 2500*time.Millisecond || ch[1].Title != "" {
		t.Errorf("chapters = %+v", ch)
	}
}

func TestParseMediaUnusableRate(t *testing.T) {
	for _, r := range []string{"0/0", "", "x", "25/0"} {
		m, err := scenes.ParseMedia([]byte(`{"format":{"duration":"N/A"},"streams":[{"avg_frame_rate":"` + r + `"}]}`))
		if err != nil || m.FPS != 0 || m.Duration != 0 {
			t.Errorf("rate %q: %+v, err %v; want zeros, nil", r, m, err)
		}
	}
	if _, err := scenes.ParseMedia([]byte("100")); err == nil {
		t.Error("non-JSON parsed")
	}
}
