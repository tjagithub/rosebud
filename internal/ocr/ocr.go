// Package ocr reads the text of a film's bitmap subtitles. It lists the
// subtitle events, draws them onto pages, and has a model read each page.
package ocr

import "time"

// Event is one subtitle shown on screen for the half-open range [Start, End).
type Event struct{ Start, End time.Duration }

// Issue is something the stage set aside instead of failing on: Ref says what
// it was, such as "line 3", and Reason why it was not used.
type Issue struct{ Ref, Reason string }
