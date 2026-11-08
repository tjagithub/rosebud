package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestReporterPlainWritesOneLinePerTenPercent(t *testing.T) {
	var b bytes.Buffer
	r := newReporter(&b, false)
	for n := int64(0); n <= 200; n++ {
		r.report("subs", n, 200)
	}
	r.report("ocr", 1, 1)
	r.finish()
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != 12 || lines[0] != "subs: 0/200" || lines[1] != "subs: 20/200" || lines[10] != "subs: 200/200" || lines[11] != "ocr: 1/1" {
		t.Errorf("got %d lines: %q", len(lines), lines)
	}
}

func TestReporterPlainStillSaysSomethingForUnknownTotals(t *testing.T) {
	var b bytes.Buffer
	r := newReporter(&b, false)
	r.report("scenes", 1, 0)
	r.report("scenes", 2, 0)
	if got := b.String(); got != "scenes: 1/0\n" {
		t.Errorf("got %q", got)
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
