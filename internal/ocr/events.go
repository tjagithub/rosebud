package ocr

import (
	"bufio"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ParseEvents reads ffprobe's "pts_time,duration_time" csv lines into events
// ordered by start. Lines it cannot use come back as issues; blank ones do not.
func ParseEvents(r io.Reader) ([]Event, []Issue, error) {
	var out []Event
	var issues []Issue
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		ref := "line " + strconv.Itoa(n)
		f := strings.Split(line, ",")
		if len(f) < 2 {
			issues = append(issues, Issue{ref, "not a start,duration pair"})
			continue
		}
		start, ok1 := seconds(f[0])
		dur, ok2 := seconds(f[1])
		switch {
		case !ok1 || !ok2:
			issues = append(issues, Issue{ref, "not a number"})
		case dur <= 0:
			issues = append(issues, Issue{ref, "duration not positive"})
		default:
			out = append(out, Event{max(start, 0), start + dur})
		}
	}
	slices.SortStableFunc(out, func(a, b Event) int { return int(a.Start - b.Start) })
	return out, issues, sc.Err()
}

func seconds(s string) (time.Duration, bool) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return time.Duration(math.Round(f * float64(time.Second))), true
}
