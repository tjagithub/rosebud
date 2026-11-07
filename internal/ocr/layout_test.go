package ocr_test

import (
	"testing"

	"github.com/tjagithub/rosebud/internal/ocr"
)

func TestPageCount(t *testing.T) {
	for _, c := range []struct{ n, want int }{
		{0, 0}, {1, 1}, {ocr.PerPage, 1}, {ocr.PerPage + 1, 2}, {3*ocr.PerPage - 1, 3},
	} {
		if got := ocr.PageCount(c.n); got != c.want {
			t.Errorf("PageCount(%d) = %d, want %d", c.n, got, c.want)
		}
	}
}

func TestSlot(t *testing.T) {
	for _, c := range []struct{ i, page, band int }{
		{0, 0, 0},
		{1, 0, 1},
		{ocr.PerPage - 1, 0, ocr.PerPage - 1},
		{ocr.PerPage, 1, 0},
		{2*ocr.PerPage + 3, 2, 3},
	} {
		page, band := ocr.Slot(c.i)
		if page != c.page || band != c.band {
			t.Errorf("Slot(%d) = %d,%d, want %d,%d", c.i, page, band, c.page, c.band)
		}
	}
}

func TestBandsFitPage(t *testing.T) {
	if ocr.PerPage*ocr.BandH > ocr.PageH {
		t.Errorf("%d bands of %d px overflow a %d px page", ocr.PerPage, ocr.BandH, ocr.PageH)
	}
}
