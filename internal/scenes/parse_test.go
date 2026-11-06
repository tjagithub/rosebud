package scenes_test

import (
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/scenes"
)

func TestParseCuts(t *testing.T) {
	const out = `frame:0    pts:12345   pts_time:12.345
lavfi.scene_score=0.512345
frame:1    pts:90000   pts_time:90
lavfi.scene_score=0.700000
noise that is not metadata
frame:2    pts:1       pts_time:oops
`
	got, err := scenes.ParseCuts(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{12345 * time.Millisecond, 90 * time.Second}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("cuts = %v, want %v", got, want)
	}

	if got, err := scenes.ParseCuts(strings.NewReader("")); err != nil || len(got) != 0 {
		t.Errorf("empty input = %v, %v; want no cuts", got, err)
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"seconds with fraction", "5400.123000\n", 5400*time.Second + 123*time.Millisecond, false},
		{"whole seconds", "60", time.Minute, false},
		{"padding", "  7.5 \n", 7500 * time.Millisecond, false},
		{"empty", "", 0, true},
		{"not a number", "N/A\n", 0, true},
		{"zero", "0.000000", 0, true},
		{"negative", "-3", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scenes.ParseDuration(tt.in)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, %v; want %v, wantErr %v", tt.in, got, err, tt.want, tt.wantErr)
			}
		})
	}
}
