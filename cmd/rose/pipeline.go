package main

import (
	"context"
	"io"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
)

// pipeline draws a bar of labelled segments, one per stage. Text written to
// it lands above the bar, which is redrawn below.
type pipeline struct {
	mu    sync.Mutex
	w     io.Writer
	names []string
	frac  map[string]float64
	width int
	open  bool
}

func newPipeline(w io.Writer, names []string, width int) *pipeline {
	return &pipeline{w: w, names: names, frac: map[string]float64{}, width: width}
}

type pipelineKey struct{}

func withPipeline(ctx context.Context, p *pipeline) context.Context {
	return context.WithValue(ctx, pipelineKey{}, p)
}

func pipelineFrom(ctx context.Context) *pipeline {
	p, _ := ctx.Value(pipelineKey{}).(*pipeline)
	return p
}

// fillCells is how many of w cells a fraction of progress covers.
func fillCells(frac float64, w int) int {
	return min(max(int(frac*float64(w)+0.5), 0), w)
}

var (
	segFull  = lipgloss.NewStyle().Reverse(true)
	segEmpty = lipgloss.NewStyle().Faint(true)
)

// bar renders every segment; widths differ by one cell at most and sum to width.
func (p *pipeline) bar() string {
	var sb strings.Builder
	n := len(p.names)
	for i, name := range p.names {
		w := p.width*(i+1)/n - p.width*i/n
		label := []rune("[" + name + strings.Repeat(" ", w))
		cells := string(label[:w-1]) + "]"
		k := fillCells(p.frac[name], w)
		r := []rune(cells)
		sb.WriteString(segFull.Render(string(r[:k])) + segEmpty.Render(string(r[k:])))
	}
	return sb.String()
}

func (p *pipeline) draw() {
	_, _ = io.WriteString(p.w, "\r\x1b[K"+p.bar())
	p.open = true
}

// set records a stage's progress and redraws.
func (p *pipeline) set(name string, done, total int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if total > 0 {
		p.frac[name] = float64(done) / float64(total)
	}
	p.draw()
}

// Write puts b above the bar and redraws the bar below it.
func (p *pipeline) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.open {
		_, _ = io.WriteString(p.w, "\r\x1b[K")
	}
	n, err := p.w.Write(b)
	if p.open {
		p.draw()
	}
	return n, err
}

// finish ends the bar's line.
func (p *pipeline) finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.open {
		_, _ = io.WriteString(p.w, "\n")
		p.open = false
	}
}
