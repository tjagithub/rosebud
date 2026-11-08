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

const playFPS = 24

// playback is a film playing; player.Playback satisfies it.
type playback interface {
	Out() <-chan player.Frame
	Stop()
	Wait() error
}

// startFunc starts playing file from a film time, in frames of w by h pixels.
type startFunc func(file string, from time.Duration, w, h int) (playback, error)

func startPlayer(file string, from time.Duration, w, h int) (playback, error) {
	return player.Play(context.Background(), player.Spec{File: file, From: from, W: w, H: h, FPS: playFPS})
}

// playState is a playback in progress in the scene screen.
type playState struct {
	pb  playback
	seq int
	at  time.Duration
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

// next waits for the picture after the last of playback seq.
func (d *detailScreen) next(seq int, pb playback) tea.Cmd {
	id, box, w, h := d.fr.id, d.fr.box, 0, 0
	w, h = framePixels(box)
	return func() tea.Msg {
		f, ok := <-pb.Out()
		if !ok {
			return playFrameMsg{Seq: seq}
		}
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for i, j := 0, 0; i+2 < len(f.Pix) && j < len(img.Pix); i, j = i+3, j+4 {
			img.Pix[j], img.Pix[j+1], img.Pix[j+2], img.Pix[j+3] = f.Pix[i], f.Pix[i+1], f.Pix[i+2], 255
		}
		data, cols, rows := encodeImage(img, id, box, w)
		return playFrameMsg{Seq: seq, At: f.At, Data: data, Cols: cols, Rows: rows}
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
	d.play = &playState{pb: pb, seq: d.seq, at: from}
	d.fr.want = "play"
	return d.next(d.seq, pb)
}

// stopPlay ends playback and returns to the scene on show.
func (d *detailScreen) stopPlay() tea.Cmd {
	p := d.play
	if p == nil {
		return nil
	}
	d.play = nil
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
	return tea.Batch(tea.Raw(msg.Data), d.next(p.seq, p.pb))
}
