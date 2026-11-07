package ocr_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/ocr"
)

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func TestParseEvents(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		want   []ocr.Event
		issues []string
	}{
		{"empty", "", nil, nil},
		{"one", "104.354000,2.162000\n", []ocr.Event{{ms(104354), ms(106516)}}, nil},
		{
			"sorted by start",
			"110.152,1.741\n104.354,2.162\n",
			[]ocr.Event{{ms(104354), ms(106516)}, {ms(110152), ms(111893)}},
			nil,
		},
		{"a trailing comma and CRLF", "1.5,2,\r\n", []ocr.Event{{ms(1500), ms(3500)}}, nil},
		{
			"a missing duration is set aside", "1,N/A\n2,\n3,1\n",
			[]ocr.Event{{sec(3), sec(4)}},
			[]string{"line 1: not a number", "line 2: not a number"},
		},
		{
			"a zero or negative duration is set aside", "1,0\n2,-1\n3,1\n",
			[]ocr.Event{{sec(3), sec(4)}},
			[]string{"line 1: duration not positive", "line 2: duration not positive"},
		},
		{"a negative start is clamped", "-0.02,1\n", []ocr.Event{{0, ms(980)}}, nil},
		{
			"junk is set aside and blank lines are ignored", "\nabc\nNaN,1\n1,Inf\n5,1\n",
			[]ocr.Event{{sec(5), sec(6)}},
			[]string{"line 2: not a start,duration pair", "line 3: not a number", "line 4: not a number"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, issues, err := ocr.ParseEvents(strings.NewReader(tt.in))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("ParseEvents = %v, want %v", got, tt.want)
			}
			var gotIssues []string
			for _, i := range issues {
				gotIssues = append(gotIssues, i.Ref+": "+i.Reason)
			}
			if !slices.Equal(gotIssues, tt.issues) {
				t.Errorf("issues = %q, want %q", gotIssues, tt.issues)
			}
		})
	}
}

func sec(n int) time.Duration { return time.Duration(n) * time.Second }
