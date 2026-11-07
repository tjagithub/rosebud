package ocr

import (
	"context"
	"image"
)

// Reader turns page images into the text blocks found on each. Adapters for
// other OCR models can sit behind it and parse their own output.
type Reader interface {
	Read(ctx context.Context, pages []image.Image) ([][]Block, error)
}
