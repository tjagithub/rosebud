package ocr_test

import (
	"slices"
	"testing"

	"github.com/tjagithub/rosebud/internal/ocr"
)

func TestPartition(t *testing.T) {
	events := []ocr.Event{
		{sec(1), sec(1) + ms(60)},  // exactly the minimum: kept
		{sec(2), sec(2) + ms(59)},  // too short
		{sec(3), sec(5)},           // kept
		{ms(5625620), ms(5625654)}, // the 34 ms stub at the end of Sonatine
	}
	valid, issues := ocr.Partition(events)
	if want := []ocr.Event{events[0], events[2]}; !slices.Equal(valid, want) {
		t.Errorf("valid = %v, want %v", valid, want)
	}
	var got []string
	for _, i := range issues {
		got = append(got, i.Ref+": "+i.Reason)
	}
	want := []string{
		"event at 2.000s: shorter than 60ms (59ms)",
		"event at 5625.620s: shorter than 60ms (34ms)",
	}
	if !slices.Equal(got, want) {
		t.Errorf("issues = %q, want %q", got, want)
	}
	if len(valid)+len(issues) != len(events) {
		t.Error("an event was neither kept nor reported")
	}
}

func TestPartitionNothing(t *testing.T) {
	if v, i := ocr.Partition(nil); len(v) != 0 || len(i) != 0 {
		t.Errorf("got %v, %v", v, i)
	}
}
