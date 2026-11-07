package ocr

import (
	"fmt"
	"time"
)

// MinEvent is the shortest event worth reading. Anything shorter is gone
// before a video frame can show it, and ffmpeg may emit no frame for it.
const MinEvent = 60 * time.Millisecond

// Partition splits events into those to read and issues for those set aside,
// so every event is either kept or reported.
func Partition(events []Event) (valid []Event, issues []Issue) {
	for _, e := range events {
		if d := e.End - e.Start; d < MinEvent {
			issues = append(issues, Issue{
				Ref:    fmt.Sprintf("event at %.3fs", e.Start.Seconds()),
				Reason: fmt.Sprintf("shorter than %v (%v)", MinEvent, d),
			})
			continue
		}
		valid = append(valid, e)
	}
	return valid, issues
}
