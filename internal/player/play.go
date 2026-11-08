package player

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// Spec says what to play.
type Spec struct {
	File    string
	From    time.Duration
	W, H    int     // size of the frames to produce, in pixels
	FPS     float64 // frames per second to show
	NoAudio bool
}

// Playback is a film playing. Frames closes when it ends or is stopped.
type Playback struct {
	Frames <-chan Frame
	stop   context.CancelFunc
	cmds   []*exec.Cmd
	once   sync.Once
	err    error
}

// Play starts the video, and the sound unless NoAudio, together.
func Play(ctx context.Context, s Spec) (*Playback, error) {
	if s.File == "" || s.W <= 0 || s.H <= 0 || s.FPS <= 0 {
		return nil, fmt.Errorf("player: bad spec %+v", s)
	}
	ctx, cancel := context.WithCancel(ctx)
	video := exec.CommandContext(ctx, "ffmpeg", VideoArgs(s.File, s.From, s.W, s.H, s.FPS)...)
	video.WaitDelay = 2 * time.Second
	out, err := video.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("player: %w", err)
	}
	p := &Playback{stop: cancel, cmds: []*exec.Cmd{video}}
	if err := video.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("player: ffmpeg: %w", err)
	}
	if !s.NoAudio {
		audio := exec.CommandContext(ctx, "ffplay", AudioArgs(s.File, s.From)...)
		audio.WaitDelay = 2 * time.Second
		if err := audio.Start(); err != nil {
			cancel()
			_ = video.Wait()
			return nil, fmt.Errorf("player: ffplay: %w", err)
		}
		p.cmds = append(p.cmds, audio)
	}
	start := time.Now()
	p.Frames = Frames(ctx, out, s.W, s.H, s.FPS, s.From,
		func() time.Duration { return time.Since(start) },
		func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-t.C:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	return p, nil
}

// Stop ends the video and the sound. It is safe to call more than once.
func (p *Playback) Stop() { p.stop() }

// Wait reaps the child processes, once Frames has closed. A stop is no error.
func (p *Playback) Wait() error {
	p.once.Do(func() {
		for _, c := range p.cmds {
			if err := c.Wait(); err != nil && !stopped(err) {
				p.err = errors.Join(p.err, fmt.Errorf("player: %s: %w", c.Path, err))
			}
		}
	})
	return p.err
}

// stopped is true for a process that was killed by Stop, as opposed to failing.
func stopped(err error) bool {
	var x *exec.ExitError
	return errors.As(err, &x) && !x.Exited()
}
