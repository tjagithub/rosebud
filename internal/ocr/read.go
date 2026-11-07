package ocr

import (
	"context"
	"fmt"
	"image"
	"sort"
	"strconv"
	"strings"

	"github.com/tjagithub/rosebud/internal/subs"
)

// ReadEvents stacks frames PerPage to a page, reads the pages and returns the
// text of each frame. Frames with no text come back empty, with an issue.
func ReadEvents(ctx context.Context, rd Reader, frames []image.Image) ([]string, []Issue, error) {
	if len(frames) == 0 {
		return nil, nil, nil
	}
	out, err := readFrames(ctx, rd, frames)
	if err != nil {
		return nil, nil, err
	}
	clearTables(out)
	// Every frame has ink, so an empty read is a model miss. Re-stack the misses
	// together: the new layout is a new cache key and a fresh chance.
	var miss []int
	for i, s := range out {
		if s == "" {
			miss = append(miss, i)
		}
	}
	if len(miss) > 0 && len(miss) < len(frames) {
		sub := make([]image.Image, len(miss))
		for k, i := range miss {
			sub[k] = frames[i]
		}
		retry, err := readFrames(withoutProgress(ctx), rd, sub)
		if err != nil {
			return nil, nil, err
		}
		clearTables(retry)
		for k, i := range miss {
			out[i] = retry[k]
		}
	}
	var issues []Issue
	for i, s := range out {
		if s == "" {
			issues = append(issues, Issue{"frame " + strconv.Itoa(i), "no text read"})
		}
	}
	return out, issues, nil
}

// readFrames stacks frames into pages, reads them and splits the text by band.
func readFrames(ctx context.Context, rd Reader, frames []image.Image) ([]string, error) {
	pages := make([]image.Image, 0, PageCount(len(frames)))
	for i := 0; i < len(frames); i += PerPage {
		pages = append(pages, Page(frames[i:min(i+PerPage, len(frames))]))
	}
	blocks, err := rd.Read(ctx, pages)
	if err != nil {
		return nil, err
	}
	if len(blocks) != len(pages) {
		return nil, fmt.Errorf("ocr: reader returned %d pages for %d", len(blocks), len(pages))
	}
	var out []string
	for p := range pages {
		n := min(PerPage, len(frames)-p*PerPage)
		out = append(out, ByBand(blocks[p], PageH, BandH, n)...)
	}
	return out, nil
}

// Assign joins the text of the events starting in each scene, by scene index.
// Events starting outside every scene are reported. Scenes must be sorted.
func Assign(scenes []subs.Span, events []Event, texts []string) (map[int]string, []Issue) {
	parts := map[int][]string{}
	var issues []Issue
	for i, e := range events {
		j := sort.Search(len(scenes), func(j int) bool { return scenes[j].End > e.Start })
		switch {
		case j == len(scenes) || e.Start < scenes[j].Start:
			issues = append(issues, Issue{fmt.Sprintf("event at %.3fs", e.Start.Seconds()), "overlaps no scene"})
		case i < len(texts) && texts[i] != "":
			parts[j] = append(parts[j], texts[i])
		}
	}
	out := make(map[int]string, len(parts))
	for j, p := range parts {
		out[j] = strings.Join(p, "\n")
	}
	return out, issues
}

// isTable spots the markdown table the model makes up when a frame has no real text.
func isTable(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "|") && strings.Contains(s, "---")
}

func clearTables(texts []string) {
	for i, s := range texts {
		if isTable(s) {
			texts[i] = ""
		}
	}
}
