package ocr

import (
	"image"
	"image/color"
)

const (
	margin    = 10 // kept around the text when cropping a frame
	padX      = 60 // where a crop is pasted within its band
	padY      = 30
	litCutoff = 64 // luma at or above this counts as subtitle ink
)

// Page draws white-on-black subtitle frames as black-on-white crops, one per
// band down a blank page. Frames past the last band are ignored.
func Page(frames []image.Image) *image.Gray {
	page := image.NewGray(image.Rect(0, 0, PageW, PageH))
	for i := range page.Pix {
		page.Pix[i] = 0xff
	}
	for i, f := range frames {
		if i >= PerPage {
			break
		}
		box := ink(f)
		if box.Empty() {
			continue
		}
		box = box.Inset(-margin).Intersect(f.Bounds())
		band := image.Rect(0, i*BandH, PageW, (i+1)*BandH)
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				dst := image.Pt(padX+x-box.Min.X, i*BandH+padY+y-box.Min.Y)
				if !dst.In(band) {
					continue
				}
				page.SetGray(dst.X, dst.Y, color.Gray{255 - luma(f, x, y)})
			}
		}
	}
	return page
}

// ink is the bounding box of the lit pixels in f.
func ink(f image.Image) image.Rectangle {
	var box image.Rectangle
	b := f.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if luma(f, x, y) < litCutoff {
				continue
			}
			box = box.Union(image.Rect(x, y, x+1, y+1))
		}
	}
	return box
}

func luma(f image.Image, x, y int) uint8 { return color.GrayModel.Convert(f.At(x, y)).(color.Gray).Y }
