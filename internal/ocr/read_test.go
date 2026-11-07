package ocr_test

import (
	"context"
	"errors"
	"image"
	"slices"
	"testing"

	"github.com/tjagithub/rosebud/internal/ocr"
	"github.com/tjagithub/rosebud/internal/subs"
)

// bandReader answers each page with one block per band named in texts.
type bandReader struct {
	texts [][]string // per page, per band; "" means no block
	err   error
	pages int
}

func (r *bandReader) Read(_ context.Context, pages []image.Image) ([][]ocr.Block, error) {
	r.pages = len(pages)
	if r.err != nil {
		return nil, r.err
	}
	out := make([][]ocr.Block, len(pages))
	for p := range pages {
		for b, s := range r.texts[p] {
			if s != "" {
				top := (float64(b) + 0.1) * ocr.BandH / ocr.PageH
				out[p] = append(out[p], ocr.Block{Top: top, Text: s})
			}
		}
	}
	return out, nil
}

func frames(n int) []image.Image {
	out := make([]image.Image, n)
	for i := range out {
		out[i] = image.NewGray(image.Rect(0, 0, 4, 4))
	}
	return out
}

func TestReadEventsSpansPages(t *testing.T) {
	texts := [][]string{{"a", "b", "", "d", "e", "f", "g", "h"}, {"i", "j"}}
	rd := &bandReader{texts: texts}
	got, issues, err := ocr.ReadEvents(t.Context(), rd, frames(10))
	if err != nil {
		t.Fatal(err)
	}
	if rd.pages != 2 {
		t.Errorf("sent %d pages, want 2", rd.pages)
	}
	want := []string{"a", "b", "", "d", "e", "f", "g", "h", "i", "j"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(issues) != 1 || issues[0].Ref != "frame 2" || issues[0].Reason != "no text read" {
		t.Errorf("issues = %+v", issues)
	}
}

func TestReadEventsErrorsAndEmpty(t *testing.T) {
	boom := errors.New("boom")
	if _, _, err := ocr.ReadEvents(t.Context(), &bandReader{err: boom}, frames(1)); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
	if got, issues, err := ocr.ReadEvents(t.Context(), &bandReader{}, nil); err != nil || len(got) != 0 || len(issues) != 0 {
		t.Errorf("empty: %v %v %v", got, issues, err)
	}
	short := readerFunc(func(context.Context, []image.Image) ([][]ocr.Block, error) { return nil, nil })
	if _, _, err := ocr.ReadEvents(t.Context(), short, frames(2)); err == nil {
		t.Error("reader returned too few pages: no error")
	}
}

type readerFunc func(context.Context, []image.Image) ([][]ocr.Block, error)

func (f readerFunc) Read(ctx context.Context, p []image.Image) ([][]ocr.Block, error) {
	return f(ctx, p)
}

func TestAssign(t *testing.T) {
	scenes := []subs.Span{{Start: sec(0), End: sec(10)}, {Start: sec(10), End: sec(20)}}
	events := []ocr.Event{{Start: sec(1), End: sec(2)}, {Start: sec(5), End: sec(6)}, {Start: sec(12), End: sec(13)}, {Start: sec(30), End: sec(31)}, {Start: sec(15), End: sec(16)}}
	texts := []string{"one", "two", "three", "late", ""}
	got, issues := ocr.Assign(scenes, events, texts)
	if got[0] != "one\ntwo" || got[1] != "three" {
		t.Errorf("got %v", got)
	}
	if len(issues) != 1 || issues[0].Ref != "event at 30.000s" || issues[0].Reason != "overlaps no scene" {
		t.Errorf("issues = %+v", issues)
	}
}
