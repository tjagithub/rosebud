package scenes

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
)

// probeMedia are the ffprobe arguments whose JSON ParseMedia reads.
var probeMedia = []string{"-v", "error", "-select_streams", "v:0", "-show_chapters",
	"-show_entries", "stream=avg_frame_rate:chapter=start_time:chapter_tags=title", "-of", "json"}

// ParseMedia reads the frame rate and chapters from ffprobe's JSON. A missing
// or unusable rate is 0.
func ParseMedia(out []byte) (fps float64, chapters []core.Chapter, err error) {
	var p struct {
		Streams []struct {
			AvgFrameRate string `json:"avg_frame_rate"`
		}
		Chapters []struct {
			StartTime string `json:"start_time"`
			Tags      map[string]string
		}
	}
	if err := json.Unmarshal(out, &p); err != nil {
		return 0, nil, fmt.Errorf("scenes: media info: %w", err)
	}
	if len(p.Streams) > 0 {
		fps = parseRate(p.Streams[0].AvgFrameRate)
	}
	for _, c := range p.Chapters {
		f, err := strconv.ParseFloat(c.StartTime, 64)
		if err != nil || f < 0 {
			continue
		}
		chapters = append(chapters, core.Chapter{Start: time.Duration(f * float64(time.Second)), Title: c.Tags["title"]})
	}
	return fps, chapters, nil
}

// parseRate reads "30000/1001" or "25"; anything unusable is 0.
func parseRate(s string) float64 {
	num, den, ok := strings.Cut(s, "/")
	n, err := strconv.ParseFloat(num, 64)
	if err != nil || n <= 0 {
		return 0
	}
	if !ok {
		return n
	}
	d, err := strconv.ParseFloat(den, 64)
	if err != nil || d <= 0 {
		return 0
	}
	return n / d
}
