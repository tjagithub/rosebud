package main

import (
	"fmt"
	"io"
	"os"

	"charm.land/bubbles/v2/progress"
	"github.com/charmbracelet/x/term"
)

// reporter shows job progress: one rewritten bar on a terminal, plain
// lines anywhere else so logs and tests stay readable.
type reporter struct {
	w    io.Writer
	live bool
	open bool // a bar is on the line and needs a newline
	bar  progress.Model

	stage string // plain mode: the stage last written,
	step  int64  // and the tenth of it reached
}

func newReporter(w io.Writer, live bool) *reporter {
	return &reporter{w: w, live: live, bar: progress.New(progress.WithWidth(24))}
}

// isTerminal reports whether w is an interactive terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}

func (r *reporter) report(stage string, done, total int64) {
	if !r.live {
		// Logs and captured output get a line per tenth, not one per tick.
		step := int64(0)
		if total > 0 {
			step = done * 10 / total
		}
		if stage == r.stage && step <= r.step && (total > 0 || done > 1) {
			return
		}
		r.stage, r.step = stage, step
		_, _ = fmt.Fprintf(r.w, "%s: %d/%d\n", stage, done, total)
		return
	}
	var pct float64
	if total > 0 {
		pct = float64(done) / float64(total)
	}
	_, _ = fmt.Fprintf(r.w, "\r\x1b[K%s %s %d/%d", stage, r.bar.ViewAs(pct), done, total)
	r.open = true
}

// finish ends the bar's line.
func (r *reporter) finish() {
	if r.open {
		_, _ = fmt.Fprintln(r.w)
		r.open = false
	}
}
