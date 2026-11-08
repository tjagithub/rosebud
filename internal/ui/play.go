package ui

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tjagithub/rosebud/internal/player"
)

const (
	playFPS     = 24
	playLead    = 24 // frames encoded ahead before playing, and while
	playWorkers = 4
)

// playback is a film playing; player.Playback satisfies it.
type playback interface {
	Out() <-chan player.Frame
	Stop()
	Wait() error
	StartAudio() error
}

// startFunc starts playing file from a film time, in frames of w by h pixels.
type startFunc func(file string, from time.Duration, w, h int) (playback, error)

func startPlayer(file string, from time.Duration, w, h int) (playback, error) {
	return player.Play(context.Background(), player.Spec{File: file, From: from, W: w, H: h, FPS: playFPS, Unpaced: true})
}

// playState is a playback in progress in the scene screen.
type playState struct {
	pb     playback
	seq    int
	at     time.Duration
	out    <-chan playFrameMsg
	cancel context.CancelFunc
}

// playFrameMsg is the next picture of playback seq, already encoded for kitty.
// Data is empty when playback has ended.
type playFrameMsg struct {
	Seq        int
	At         time.Duration
	Data       string
	Cols, Rows int
}

// sourceFile finds the film beside its bundle: Name.film holds Name.<ext>.
func sourceFile(bundle string) string {
	dir, base := filepath.Split(filepath.Clean(bundle))
	name := strings.TrimSuffix(base, filepath.Ext(base))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range ents {
		ext := filepath.Ext(e.Name())
		if e.IsDir() || ext == ".film" || strings.TrimSuffix(e.Name(), ext) != name {
			continue
		}
		return filepath.Join(dir, e.Name())
	}
	return ""
}

// framePixels is the size of the pictures to ask for, for a box of cells.
func framePixels(box size) (w, h int) {
	w = min(maxPixels, box.cols*10) &^ 1
	h = max(2, w*box.rows*2/max(box.cols, 1)) &^ 1
	return
}

// encodeFrame turns a decoded picture into the kitty upload that shows it.
func encodeFrame(f player.Frame, seq, id int, box size, w, h int) playFrameMsg {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i, j := 0, 0; i+2 < len(f.Pix) && j < len(img.Pix); i, j = i+3, j+4 {
		img.Pix[j], img.Pix[j+1], img.Pix[j+2], img.Pix[j+3] = f.Pix[i], f.Pix[i+1], f.Pix[i+2], 255
	}
	data, cols, rows := encodeImage(img, id, box, w)
	return playFrameMsg{Seq: seq, At: f.At, Data: data, Cols: cols, Rows: rows}
}

// pump encodes the frames of pb ahead, in parallel and in order, holds back
// about lead of them, then hands them out at the frame rate. A frame a whole
// step late is dropped. The channel closes when pb ends or ctx is done.
func pump(ctx context.Context, pb playback, enc func(player.Frame) playFrameMsg, fps float64, lead, workers int) <-chan playFrameMsg {
	order := make(chan chan playFrameMsg, lead)
	out := make(chan playFrameMsg)
	sem := make(chan struct{}, workers)
	read := make(chan struct{})
	go func() {
		defer close(read)
		defer close(order)
		for f := range pb.Out() {
			r := make(chan playFrameMsg, 1)
			select {
			case order <- r:
			case <-ctx.Done():
				return
			}
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			go func() {
				r <- enc(f)
				<-sem
			}()
		}
	}()
	go func() {
		defer close(out)
		for tick := time.NewTicker(10 * time.Millisecond); len(order) < cap(order) && ctx.Err() == nil; {
			select {
			case <-tick.C:
			case <-read:
				tick.Stop()
				goto play
			}
		}
	play:
		_ = pb.StartAudio()
		start, step := time.Now(), time.Duration(float64(time.Second)/fps)
		for i := 0; ; i++ {
			var r chan playFrameMsg
			var ok bool
			select {
			case r, ok = <-order:
			case <-ctx.Done():
				return
			}
			if !ok {
				return
			}
			var m playFrameMsg
			select {
			case m = <-r:
			case <-ctx.Done():
				return
			}
			late := time.Since(start) - time.Duration(i)*step
			if late >= step {
				continue
			}
			if late < 0 {
				select {
				case <-time.After(-late):
				case <-ctx.Done():
					return
				}
			}
			select {
			case out <- m:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// next waits for the picture after the last of the playback in progress.
func (d *detailScreen) next(p *playState) tea.Cmd {
	seq, out := p.seq, p.out
	return func() tea.Msg {
		if m, ok := <-out; ok {
			return m
		}
		return playFrameMsg{Seq: seq}
	}
}

// startPlay plays from the current scene. It does nothing if the film is not beside its bundle.
func (d *detailScreen) startPlay() tea.Cmd {
	file := sourceFile(d.fr.bundle)
	if file == "" || len(d.hits) == 0 || d.fr.box.cols == 0 {
		return nil
	}
	from := d.hit().Scene.Start
	w, h := framePixels(d.fr.box)
	pb, err := d.start(file, from, w, h)
	if err != nil {
		d.err = err
		d.relayout(false)
		return nil
	}
	d.seq++
	ctx, cancel := context.WithCancel(context.Background())
	seq, id, box := d.seq, d.fr.id, d.fr.box
	enc := func(f player.Frame) playFrameMsg { return encodeFrame(f, seq, id, box, w, h) }
	d.play = &playState{pb: pb, seq: seq, at: from, cancel: cancel,
		out: pump(ctx, pb, enc, playFPS, playLead, playWorkers)}
	d.fr.want = "play"
	return d.next(d.play)
}

// stopPlay ends playback and returns to the scene on show.
func (d *detailScreen) stopPlay() tea.Cmd {
	p := d.play
	if p == nil {
		return nil
	}
	d.play = nil
	p.cancel()
	p.pb.Stop()
	go func() {
		for range p.pb.Out() {
		}
		_ = p.pb.Wait()
	}()
	d.i = d.sceneAt(p.at)
	return d.reload()
}

// onPlayFrame draws a picture of the playback in progress and asks for the next.
func (d *detailScreen) onPlayFrame(msg playFrameMsg) tea.Cmd {
	p := d.play
	if p == nil || msg.Seq != p.seq {
		return nil
	}
	if msg.Data == "" && msg.Cols == 0 && msg.At == 0 {
		return d.stopPlay()
	}
	p.at = msg.At
	d.fr.shown, d.fr.cols, d.fr.rows = "play", msg.Cols, msg.Rows
	return tea.Batch(tea.Raw(msg.Data), d.next(p))
}
