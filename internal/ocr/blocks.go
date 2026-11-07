package ocr

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Block is a run of text the model found, with the page position of its top
// edge as a fraction of the page height.
type Block struct {
	Top  float64
	Text string
}

var (
	blockRE = regexp.MustCompile(`(?s)<x_[\d.]+><y_([\d.]+)>(.*?)<x_[\d.]+><y_[\d.]+><class_[^>]*>`)
	markRE  = regexp.MustCompile(`^#{1,6} `)
)

// ParseBlocks reads the model's "<x_><y_>text<x_><y_><class_>" output. Header
// marks are dropped, and so are blocks left with no text.
func ParseBlocks(s string) []Block {
	var out []Block
	for _, m := range blockRE.FindAllStringSubmatch(s, -1) {
		top, err := strconv.ParseFloat(m[1], 64)
		text := strings.TrimSpace(markRE.ReplaceAllString(strings.TrimSpace(m[2]), ""))
		if err != nil || text == "" {
			continue
		}
		out = append(out, Block{top, text})
	}
	return out
}

// ByBand gives the text of each of n events drawn as bandH-pixel bands down a
// pageH-pixel page. A block goes to the band holding its top edge.
func ByBand(blocks []Block, pageH, bandH, n int) []string {
	out := make([]string, n)
	for _, b := range blocks {
		if b.Top < 0 || b.Top > 1 || math.IsNaN(b.Top) {
			continue
		}
		i := int(b.Top * float64(pageH) / float64(bandH))
		if i >= n {
			continue
		}
		if out[i] != "" {
			out[i] += "\n"
		}
		out[i] += b.Text
	}
	return out
}
