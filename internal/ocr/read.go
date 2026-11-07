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
	pages := make([]image.Image, 0, PageCount(len(frames)))
	for i := 0; i < len(frames); i += PerPage {
		pages = append(pages, Page(frames[i:min(i+PerPage, len(frames))]))
	}
	blocks, err := rd.Read(ctx, pages)
	if err != nil {
		return nil, nil, err
	}
	if len(blocks) != len(pages) {
		return nil, nil, fmt.Errorf("ocr: reader returned %d pages for %d", len(blocks), len(pages))
	}
	var out []string
	var issues []Issue
	for p := range pages {
		n := min(PerPage, len(frames)-p*PerPage)
		for b, s := range ByBand(blocks[p], PageH, BandH, n) {
			if s == "" {
				issues = append(issues, Issue{"frame " + strconv.Itoa(p*PerPage+b), "no text read"})
			}
			out = append(out, s)
		}
	}
	return out, issues, nil
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
