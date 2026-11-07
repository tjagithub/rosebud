package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestReporterPlainWritesLines(t *testing.T) {
	var b bytes.Buffer
	r := newReporter(&b, false)
	r.report("scenes", 1, 3)
	r.report("scenes", 2, 3)
	r.finish()
	if got, want := b.String(), "scenes: 1/3\nscenes: 2/3\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReporterLiveRewritesOneLine(t *testing.T) {
	var b bytes.Buffer
	r := newReporter(&b, true)
	r.report("scenes", 0, 4)
	r.report("scenes", 2, 4)
	r.finish()
	out := b.String()
	if strings.Count(out, "\r\x1b[K") != 2 {
		t.Errorf("each update must clear the line first: %q", out)
	}
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
		t.Errorf("only finish may end the line: %q", out)
	}
	if !strings.Contains(out, "50%") || !strings.Contains(out, "2/4") {
		t.Errorf("missing percent or count: %q", out)
	}
}

func TestReporterFinishWithoutOutputWritesNothing(t *testing.T) {
	var b bytes.Buffer
	newReporter(&b, true).finish()
	if b.Len() != 0 {
		t.Errorf("got %q", b.String())
	}
}
