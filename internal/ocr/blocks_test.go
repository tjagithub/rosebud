package ocr_test

import (
	"slices"
	"testing"

	"github.com/tjagithub/rosebud/internal/ocr"
)

func TestParseBlocks(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []ocr.Block
	}{
		{"empty", "", nil},
		{"plain text", "hello", nil},
		{"one", "<x_0.0391><y_0.018>はい<x_0.1104><y_0.0461><class_Text>", []ocr.Block{{0.018, "はい"}}},
		{
			"two blocks",
			"<x_0.04><y_0.02>はい<x_0.11><y_0.05><class_Text>\n\n<x_0.04><y_0.13>あとで<x_0.36><y_0.16><class_Text>",
			[]ocr.Block{{0.02, "はい"}, {0.13, "あとで"}},
		},
		{
			"lines inside a block are kept",
			"<x_0.04><y_0.58>おい!\nさらっちまうぞ コラ ああ?<x_0.49><y_0.64><class_Text>",
			[]ocr.Block{{0.58, "おい!\nさらっちまうぞ コラ ああ?"}},
		},
		{"header marks are stripped", "<x_0.04><y_0.13>### ヤクザ 辞めたくなったな<x_0.44><y_0.16><class_Section-header>", []ocr.Block{{0.13, "ヤクザ 辞めたくなったな"}}},
		{"a title mark too", "<x_0.23><y_0.28># はい<x_0.37><y_0.33><class_Title>", []ocr.Block{{0.28, "はい"}}},
		{"an empty block is dropped", "<x_0.1><y_0.1> <x_0.2><y_0.2><class_Picture>", nil},
		{"a hash without a space is text", "<x_0.1><y_0.1>#1 ファン<x_0.2><y_0.2><class_Text>", []ocr.Block{{0.1, "#1 ファン"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ocr.ParseBlocks(tt.in); !slices.Equal(got, tt.want) {
				t.Errorf("ParseBlocks = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestByBand(t *testing.T) {
	const pageH, bandH = 2000, 200
	b := func(px float64, text string) ocr.Block { return ocr.Block{Top: px / pageH, Text: text} }
	tests := []struct {
		name   string
		blocks []ocr.Block
		n      int
		want   []string
	}{
		{"no blocks", nil, 3, []string{"", "", ""}},
		{"one per band", []ocr.Block{b(30, "a"), b(230, "b")}, 3, []string{"a", "b", ""}},
		{"a split event is joined by a newline", []ocr.Block{b(430, "x"), b(560, "y")}, 3, []string{"", "", "x\ny"}},
		{"blocks past the last band are dropped", []ocr.Block{b(30, "a"), b(430, "z")}, 2, []string{"a", ""}},
		{"a block on a band edge goes to the lower band", []ocr.Block{b(200, "a")}, 2, []string{"", "a"}},
		{"a bad position is dropped", []ocr.Block{{Top: -1, Text: "a"}, {Top: 2, Text: "b"}}, 2, []string{"", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ocr.ByBand(tt.blocks, pageH, bandH, tt.n); !slices.Equal(got, tt.want) {
				t.Errorf("ByBand = %q, want %q", got, tt.want)
			}
		})
	}
}
