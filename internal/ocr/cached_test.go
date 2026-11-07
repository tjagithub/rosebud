package ocr_test

import (
	"context"
	"errors"
	"image"
	"image/color"
	"slices"
	"testing"

	"github.com/tjagithub/rosebud/internal/cache"
	"github.com/tjagithub/rosebud/internal/ocr"
)

// pageWith is a small grey page whose single pixel makes it unique.
func pageWith(v uint8) image.Image {
	g := image.NewGray(image.Rect(0, 0, 4, 4))
	g.SetGray(1, 1, color.Gray{v})
	return g
}

// echo answers each page with one block naming its pixel, and logs batches.
type echo struct {
	batches [][]int
	failOn  int // batch number to fail, counting from 1; 0 never
}

func (e *echo) Read(_ context.Context, pages []image.Image) ([][]ocr.Block, error) {
	var seen []int
	out := make([][]ocr.Block, len(pages))
	for i, p := range pages {
		v := int(p.(*image.Gray).GrayAt(1, 1).Y)
		seen = append(seen, v)
		out[i] = []ocr.Block{{Top: 0.01, Text: string(rune('a' + v))}}
	}
	e.batches = append(e.batches, seen)
	if e.failOn == len(e.batches) {
		return nil, errors.New("boom")
	}
	return out, nil
}

func texts(bs [][]ocr.Block) []string {
	var out []string
	for _, b := range bs {
		out = append(out, b[0].Text)
	}
	return out
}

func newCached(t *testing.T, in ocr.Reader, salt string, batch int) ocr.Cached {
	return ocr.Cached{Inner: in, Dir: cache.Dir{Path: t.TempDir()}, Salt: salt, Batch: batch}
}

func TestCachedReadsEachPageOnce(t *testing.T) {
	in := &echo{}
	c := newCached(t, in, "m1", 2)
	pages := []image.Image{pageWith(0), pageWith(1), pageWith(2)}
	got, err := c.Read(t.Context(), pages)
	if err != nil || !slices.Equal(texts(got), []string{"a", "b", "c"}) {
		t.Fatalf("first: %v %v", texts(got), err)
	}
	if len(in.batches) != 2 || len(in.batches[0]) != 2 {
		t.Errorf("batches = %v, want [2 1]", in.batches)
	}
	in.batches = nil
	again, err := c.Read(t.Context(), []image.Image{pageWith(2), pageWith(0)})
	if err != nil || !slices.Equal(texts(again), []string{"c", "a"}) || len(in.batches) != 0 {
		t.Errorf("second: %v %v, inner called %v", texts(again), err, in.batches)
	}
}

func TestCachedOnlyReadsMisses(t *testing.T) {
	in := &echo{}
	c := newCached(t, in, "m1", 8)
	_, _ = c.Read(t.Context(), []image.Image{pageWith(1)})
	in.batches = nil
	got, err := c.Read(t.Context(), []image.Image{pageWith(0), pageWith(1), pageWith(2)})
	if err != nil || !slices.Equal(texts(got), []string{"a", "b", "c"}) {
		t.Fatalf("%v %v", texts(got), err)
	}
	if len(in.batches) != 1 || !slices.Equal(in.batches[0], []int{0, 2}) {
		t.Errorf("batches = %v, want only pages 0 and 2", in.batches)
	}
}

func TestCachedKeepsFinishedBatchesWhenALaterOneFails(t *testing.T) {
	in := &echo{failOn: 2}
	c := newCached(t, in, "m1", 2)
	pages := []image.Image{pageWith(0), pageWith(1), pageWith(2), pageWith(3)}
	if _, err := c.Read(t.Context(), pages); err == nil {
		t.Fatal("no error")
	}
	in.failOn, in.batches = 0, nil
	if _, err := c.Read(t.Context(), pages); err != nil {
		t.Fatal(err)
	}
	if len(in.batches) != 1 || !slices.Equal(in.batches[0], []int{2, 3}) {
		t.Errorf("rerun read %v, want only the failed batch", in.batches)
	}
}

func TestCachedSaltSeparatesModels(t *testing.T) {
	in := &echo{}
	c := newCached(t, in, "m1", 8)
	_, _ = c.Read(t.Context(), []image.Image{pageWith(0)})
	c.Salt = "m2"
	in.batches = nil
	_, _ = c.Read(t.Context(), []image.Image{pageWith(0)})
	if len(in.batches) != 1 {
		t.Error("a different model reused another's result")
	}
}

func TestCachedIgnoresCorruptEntries(t *testing.T) {
	in := &echo{}
	c := newCached(t, in, "m1", 8)
	_, _ = c.Read(t.Context(), []image.Image{pageWith(0)})
	for _, k := range keysIn(t, c.Dir) {
		if err := c.Dir.Put(k, []byte("{not json")); err != nil {
			t.Fatal(err)
		}
	}
	in.batches = nil
	got, err := c.Read(t.Context(), []image.Image{pageWith(0)})
	if err != nil || texts(got)[0] != "a" || len(in.batches) != 1 {
		t.Errorf("%v %v %v", texts(got), err, in.batches)
	}
}

func TestCachedStopsWhenCancelled(t *testing.T) {
	in := &echo{}
	c := newCached(t, in, "m1", 1)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.Read(ctx, []image.Image{pageWith(0), pageWith(1)}); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
	if len(in.batches) != 0 {
		t.Error("read after cancel")
	}
}

func TestCachedRejectsBadSetup(t *testing.T) {
	for _, c := range []ocr.Cached{{Dir: cache.Dir{Path: t.TempDir()}, Salt: "s", Batch: 1}, {Inner: &echo{}, Salt: "s", Batch: 1}, {Inner: &echo{}, Dir: cache.Dir{Path: t.TempDir()}, Batch: 1}, {Inner: &echo{}, Dir: cache.Dir{Path: t.TempDir()}, Salt: "s"}} {
		if _, err := c.Read(t.Context(), []image.Image{pageWith(0)}); err == nil {
			t.Errorf("%+v: no error", c)
		}
	}
}

func TestCachedReportsProgressCountingHits(t *testing.T) {
	c := newCached(t, &echo{}, "m1", 1)
	if _, err := c.Read(t.Context(), []image.Image{pageWith(0)}); err != nil {
		t.Fatal(err)
	}
	var seen [][2]int64
	ctx := ocr.WithProgress(t.Context(), func(done, total int64) { seen = append(seen, [2]int64{done, total}) })
	if _, err := c.Read(ctx, []image.Image{pageWith(0), pageWith(1), pageWith(2)}); err != nil {
		t.Fatal(err)
	}
	want := [][2]int64{{1, 3}, {2, 3}, {3, 3}}
	if !slices.Equal(seen, want) {
		t.Errorf("progress = %v, want %v", seen, want)
	}
}

func TestRetryReadDoesNotReportProgress(t *testing.T) {
	var calls int
	ctx := ocr.WithProgress(t.Context(), func(int64, int64) { calls++ })
	c := newCached(t, &echo{}, "m1", 1)
	rd := readerFunc(func(ctx context.Context, p []image.Image) ([][]ocr.Block, error) {
		got, err := c.Read(ctx, p)
		if len(got) > 0 {
			got[0] = nil // first frame always reads empty
		}
		return got, err
	})
	if _, _, err := ocr.ReadEvents(ctx, rd, frames(2)); err != nil {
		t.Fatal(err)
	}
	if calls != 2 { // one initial report and one batch, none for the retry
		t.Errorf("progress calls = %d, want 2", calls)
	}
}
