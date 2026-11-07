package main

import (
	"testing"
	"time"
)

func TestParseRange(t *testing.T) {
	f, to, err := parseRange("1m:2m30s")
	if err != nil || f != time.Minute || to != 150*time.Second {
		t.Errorf("got %v %v %v", f, to, err)
	}
	if f, to, err = parseRange(":30s"); err != nil || f != 0 || to != 30*time.Second {
		t.Errorf("open start: %v %v %v", f, to, err)
	}
	for _, bad := range []string{"30", "x:1m", "2m:1m", "-1s:5s"} {
		if _, _, err := parseRange(bad); err == nil {
			t.Errorf("%q must fail", bad)
		}
	}
}

func TestParseMasks(t *testing.T) {
	ms, err := parseMasks("200,60,0,0; iw*0.2,40,10,10")
	if err != nil || len(ms) != 2 {
		t.Fatalf("got %v %v", ms, err)
	}
	if _, err := parseMasks("1,2,3"); err == nil {
		t.Error("short mask must fail")
	}
}
