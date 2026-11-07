package ocr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"image"
	"strconv"

	"github.com/tjagithub/rosebud/internal/cache"
)

// Cached reads each page once. Misses go to Inner Batch pages at a time and
// each batch is saved before the next, so a cancelled run keeps its progress.
type Cached struct {
	Inner Reader
	Dir   cache.Dir
	Salt  string // names the model and its setup; a change misses every entry
	Batch int
}

// Read returns the blocks of each page. Cache failures only cost a re-read.
func (c Cached) Read(ctx context.Context, pages []image.Image) ([][]Block, error) {
	if c.Inner == nil || c.Dir.Path == "" || c.Salt == "" || c.Batch <= 0 {
		return nil, errors.New("ocr: cached reader needs Inner, Dir, Salt and a positive Batch")
	}
	out := make([][]Block, len(pages))
	keys := make([]string, len(pages))
	var miss []int
	for i, p := range pages {
		keys[i] = pageKey(c.Salt, p)
		if b, ok, _ := c.Dir.Get(keys[i]); ok && json.Unmarshal(b, &out[i]) == nil {
			continue
		}
		miss = append(miss, i)
	}
	for len(miss) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch := miss[:min(c.Batch, len(miss))]
		miss = miss[len(batch):]
		in := make([]image.Image, len(batch))
		for j, i := range batch {
			in[j] = pages[i]
		}
		got, err := c.Inner.Read(ctx, in)
		if err != nil {
			return nil, err
		}
		if len(got) != len(in) {
			return nil, fmt.Errorf("ocr: reader returned %d pages for %d", len(got), len(in))
		}
		for j, i := range batch {
			out[i] = got[j]
			if b, err := json.Marshal(got[j]); err == nil {
				_ = c.Dir.Put(keys[i], b)
			}
		}
	}
	return out, nil
}

// pageKey hashes the salt and the page's grey pixels.
func pageKey(salt string, p image.Image) string {
	h := sha256.New()
	_, _ = h.Write([]byte(salt))
	b := p.Bounds()
	_, _ = h.Write([]byte("\x00" + strconv.Itoa(b.Dx()) + "x" + strconv.Itoa(b.Dy()) + "\x00"))
	if g, ok := p.(*image.Gray); ok {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			o := g.PixOffset(b.Min.X, y)
			_, _ = h.Write(g.Pix[o : o+b.Dx()])
		}
	} else {
		hashSlow(h, p)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func hashSlow(h hash.Hash, p image.Image) {
	b := p.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _ = h.Write([]byte{luma(p, x, y)})
		}
	}
}
