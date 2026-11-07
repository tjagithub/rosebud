package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestPipelineBarHasOneLabelledSegmentPerStage(t *testing.T) {
	p := newPipeline(&bytes.Buffer{}, []string{"scenes", "ocr", "translate"}, 60)
	line := ansi.Strip(p.bar())
	if ansi.StringWidth(line) != 60 {
		t.Errorf("width = %d, want 60: %q", ansi.StringWidth(line), line)
	}
	for _, name := range []string{"[scenes", "[ocr", "[translate"} {
		if !strings.Contains(line, name) {
			t.Errorf("bar lacks %q: %q", name, line)
		}
	}
}

func TestPipelineFillFollowsProgress(t *testing.T) {
	for _, tt := range []struct {
		frac float64
		w    int
		want int
	}{{0, 10, 0}, {0.5, 10, 5}, {1, 10, 10}, {2, 10, 10}, {-1, 10, 0}} {
		if got := fillCells(tt.frac, tt.w); got != tt.want {
			t.Errorf("fillCells(%v, %d) = %d, want %d", tt.frac, tt.w, got, tt.want)
		}
	}
}

func TestPipelineWriterKeepsTextAboveTheBar(t *testing.T) {
	var out bytes.Buffer
	p := newPipeline(&out, []string{"a", "b"}, 20)
	p.set("a", 1, 2)
	out.Reset()
	if _, err := p.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.HasPrefix(s, "\r\x1b[Khello\n") || !strings.Contains(s[len("\r\x1b[Khello\n"):], "[a") {
		t.Errorf("text must replace the bar line, then the bar is redrawn: %q", s)
	}
}

func TestPipelineRoundTripsThroughContext(t *testing.T) {
	p := newPipeline(&bytes.Buffer{}, []string{"slow"}, 20)
	if pipelineFrom(withPipeline(context.Background(), p)) != p || pipelineFrom(context.Background()) != nil {
		t.Error("pipeline must round-trip through the context")
	}
}
