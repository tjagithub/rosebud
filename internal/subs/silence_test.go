package subs_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/subs"
)

func TestParseSilences(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	tests := []struct {
		name  string
		in    string
		total time.Duration
		want  []subs.Span
	}{
		{"empty", "", sec(10), nil},
		{
			"pairs",
			"lavfi.silence_start=0\nlavfi.silence_end=0.939\nlavfi.silence_duration=0.939\n" +
				"lavfi.silence_start=2.5\nlavfi.silence_end=4.47\n",
			sec(10),
			[]subs.Span{{0, ms(939)}, {ms(2500), ms(4470)}},
		},
		{
			"metadata frame lines are skipped",
			"frame:7    pts:7168    pts_time:0.448\nlavfi.silence_start=1\nframe:9 pts:1 pts_time:2\nlavfi.silence_end=2\n",
			sec(10),
			[]subs.Span{{sec(1), sec(2)}},
		},
		{"an open silence runs to the end", "lavfi.silence_start=8\n", sec(10), []subs.Span{{sec(8), sec(10)}}},
		{"an end without a start is ignored", "lavfi.silence_end=3\nlavfi.silence_start=5\nlavfi.silence_end=6\n", sec(10), []subs.Span{{sec(5), sec(6)}}},
		{"a negative start is clamped", "lavfi.silence_start=-0.02\nlavfi.silence_end=1\n", sec(10), []subs.Span{{0, sec(1)}}},
		{"an end past the film is clamped", "lavfi.silence_start=9\nlavfi.silence_end=12\n", sec(10), []subs.Span{{sec(9), sec(10)}}},
		{"a silence wholly past the film is dropped", "lavfi.silence_start=11\nlavfi.silence_end=12\n", sec(10), nil},
		{"an empty silence is dropped", "lavfi.silence_start=4\nlavfi.silence_end=4\n", sec(10), nil},
		{"bad numbers are skipped", "lavfi.silence_start=abc\nlavfi.silence_start=NaN\nlavfi.silence_start=1\nlavfi.silence_end=2\n", sec(10), []subs.Span{{sec(1), sec(2)}}},
		{"a NaN start is not a start", "lavfi.silence_start=NaN\nlavfi.silence_end=2\n", sec(10), nil},
		{"an infinite start is not a start", "lavfi.silence_start=-Inf\nlavfi.silence_end=2\n", sec(10), nil},
		{"a new start replaces an unfinished one", "lavfi.silence_start=1\nlavfi.silence_start=3\nlavfi.silence_end=4\n", sec(10), []subs.Span{{sec(3), sec(4)}}},
		{"CRLF line endings", "lavfi.silence_start=1\r\nlavfi.silence_end=2\r\n", sec(10), []subs.Span{{sec(1), sec(2)}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := subs.ParseSilences(strings.NewReader(tt.in), tt.total)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("ParseSilences = %v, want %v", got, tt.want)
			}
		})
	}
}
