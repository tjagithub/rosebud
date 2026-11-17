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
var probeMedia = []string{
	"-v", "error", "-select_streams", "v:0", "-show_chapters",
	"-show_entries", "format=duration:stream=avg_frame_rate:chapter=start_time:chapter_tags=title", "-of", "json",
}

// ParseMedia reads the length, frame rate and chapters from ffprobe's JSON. A
// missing or unusable length or rate is 0.
func ParseMedia(out []byte) (core.Media, error) {
	var p struct {
		Format struct {
			Duration string
		}
		Streams []struct {
			AvgFrameRate string `json:"avg_frame_rate"`
		}
		Chapters []struct {
			StartTime string `json:"start_time"`
			Tags      map[string]string
		}
	}
	var m core.Media
	if err := json.Unmarshal(out, &p); err != nil {
		return m, fmt.Errorf("scenes: media info: %w", err)
	}
	if d, err := strconv.ParseFloat(p.Format.Duration, 64); err == nil && d > 0 {
		m.Duration = time.Duration(d * float64(time.Second))
	}
	if len(p.Streams) > 0 {
		m.FPS = parseRate(p.Streams[0].AvgFrameRate)
	}
	for _, c := range p.Chapters {
		f, err := strconv.ParseFloat(c.StartTime, 64)
		if err != nil || f < 0 {
			continue
		}
		m.Chapters = append(m.Chapters, core.Chapter{Start: time.Duration(f * float64(time.Second)), Title: c.Tags["title"]})
	}
	return m, nil
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
