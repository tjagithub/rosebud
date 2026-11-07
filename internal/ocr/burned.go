package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/limit"
	"github.com/tjagithub/rosebud/internal/subs"
)

// Sample is the subtitle band of the video at one moment, white text on black.
type Sample struct {
	At  time.Duration
	Img *image.Gray
}

// Whiten keeps pixels at or above cutoff and blacks out the rest, which leaves
// burned-in text looking like a rendered subtitle frame.
func Whiten(img image.Image, cutoff uint8) *image.Gray {
	b := img.Bounds()
	out := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if l := luma(img, x, y); l >= cutoff {
				out.SetGray(x-b.Min.X, y-b.Min.Y, color.Gray{l})
			}
		}
	}
	return out
}

// lit counts the pixels of g that are on.
func lit(g *image.Gray) int {
	n := 0
	for _, p := range g.Pix {
		if p != 0 {
			n++
		}
	}
	return n
}

// overlap is the share of lit pixels that two same-size frames light together.
func overlap(a, b *image.Gray) float64 {
	if a.Bounds() != b.Bounds() {
		return 0
	}
	var both, either int
	for i := range a.Pix {
		x, y := a.Pix[i] != 0, b.Pix[i] != 0
		if x && y {
			both++
		}
		if x || y {
			either++
		}
	}
	if either == 0 {
		return 0
	}
	return float64(both) / float64(either)
}

// maxLitShare is the most of a band that text can light; more is a picture.
const maxLitShare = 0.25

// sameLine is how much two samples must overlap to count as one subtitle.
const sameLine = 0.4

// BurnedEvents turns samples taken step apart into subtitle events. Runs of
// samples with the same lit shape make one event, shown by its brightest sample.
func BurnedEvents(samples []Sample, step time.Duration, minInk int) ([]Event, []image.Image) {
	var events []Event
	var imgs []image.Image
	var best *image.Gray
	var bestInk int
	open := false
	for i, s := range samples {
		n := lit(s.Img)
		if n < minInk || float64(n) > maxLitShare*float64(len(s.Img.Pix)) {
			open = false
			continue
		}
		if open && overlap(samples[i-1].Img, s.Img) >= sameLine {
			events[len(events)-1].End = s.At + step
			if n > bestInk {
				best, bestInk = s.Img, n
				imgs[len(imgs)-1] = best
			}
			continue
		}
		open, best, bestInk = true, s.Img, n
		events = append(events, Event{Start: s.At, End: s.At + step})
		imgs = append(imgs, best)
	}
	return events, imgs
}

// BurnedStage reads subtitles burned into the picture: it samples the bottom
// of the video, finds the lines of text and OCRs each. The job's "file" param names the video.
// Sampling says which frames of the video are looked at.
type Sampling struct {
	FPS      int           // frames per second
	Masks    []Mask        // areas to black out first, such as a watermark
	From, To time.Duration // the part to read; zero To means to the end
}

type BurnedStage struct {
	Exec     Exec
	Store    Store
	Reader   Reader
	MinInk   int           // fewest lit pixels that count as text
	Timeout  time.Duration // for sampling the video
	Sampling Sampling
	OnIssues func([]Issue)
}

// Run is a jobs.Stage. Text is written once, at the end.
func (s BurnedStage) Run(ctx context.Context, spec core.JobSpec, progress func(done, total int64)) error {
	file := spec.Params["file"]
	if file == "" || s.Sampling.FPS <= 0 || s.Timeout <= 0 {
		return errors.New("ocr: burned-in stage needs a file, a positive FPS and a timeout")
	}
	scenes, err := Stage{Store: s.Store}.scenes(ctx, spec.Film)
	if err != nil {
		return err
	}
	samples, err := s.sample(ctx, file)
	if err != nil {
		return err
	}
	step := time.Second / time.Duration(s.Sampling.FPS)
	events, imgs := BurnedEvents(samples, step, s.MinInk)
	if len(events) == 0 {
		return core.Skip("no burned-in subtitles found in %s", file)
	}
	progress(0, int64(PageCount(len(imgs))))
	texts, issues, err := ReadEvents(WithProgress(ctx, progress), s.Reader, imgs)
	if err != nil {
		return err
	}
	spans := make([]subs.Span, len(scenes))
	for i, sc := range scenes {
		spans[i] = subs.Span{Start: sc.Start, End: sc.End}
	}
	byScene, more := Assign(spans, events, texts)
	issues = append(issues, more...)
	out := make(map[core.SceneID]string, len(byScene))
	for i, t := range byScene {
		out[scenes[i].ID] = t
	}
	if s.OnIssues != nil {
		s.OnIssues(issues)
	}
	return s.Store.SetSubtitle(ctx, spec.Film, out)
}

// sample decodes the bottom band of the video FPS times a second.
func (s BurnedStage) sample(ctx context.Context, file string) ([]Sample, error) {
	vf := "fps=" + strconv.Itoa(s.Sampling.FPS) + ","
	for _, m := range s.Sampling.Masks {
		vf += m.Filter() + ","
	}
	vf += "crop=iw:ih*0.3:0:ih*0.68,scale=1280:-2,format=gray"
	out, err := limit.Run(ctx, "ffmpeg", s.Timeout, func(ctx context.Context) ([]byte, error) {
		args := []string{"-nostdin", "-v", "error"}
		if s.Sampling.From > 0 {
			args = append(args, "-ss", fmt.Sprintf("%.3f", s.Sampling.From.Seconds()))
		}
		if s.Sampling.To > 0 {
			args = append(args, "-to", fmt.Sprintf("%.3f", s.Sampling.To.Seconds()))
		}
		args = append(args, "-i", file, "-an", "-sn", "-vf", vf, "-f", "image2pipe", "-c:v", "png", "-")
		return s.Exec(ctx, "ffmpeg", args...)
	})
	if err != nil {
		return nil, fmt.Errorf("ocr: sample %s: %w", file, err)
	}
	var samples []Sample
	r := bytes.NewReader(out)
	for r.Len() > 0 {
		img, err := png.Decode(r)
		if err != nil {
			return nil, fmt.Errorf("ocr: decode sample %d of %s: %w", len(samples), file, err)
		}
		at := s.Sampling.From + time.Duration(len(samples))*time.Second/time.Duration(s.Sampling.FPS)
		samples = append(samples, Sample{At: at, Img: Whiten(img, 200)})
	}
	return samples, nil
}

// Mask is a rectangle of the picture to black out, as ffmpeg expressions
// that may use iw and ih, such as "iw*0.2,60,0,0" for width, height, x and y.
type Mask struct{ W, H, X, Y string }

var safeExpr = regexp.MustCompile(`^[0-9a-z*+\-/.()]+$`)

// ParseMask reads "w,h,x,y", refusing anything that could be more than arithmetic.
func ParseMask(s string) (Mask, error) {
	p := strings.Split(s, ",")
	if len(p) != 4 {
		return Mask{}, fmt.Errorf("mask %q: want w,h,x,y", s)
	}
	for _, v := range p {
		if !safeExpr.MatchString(v) {
			return Mask{}, fmt.Errorf("mask %q: %q is not a number or an iw/ih expression", s, v)
		}
	}
	return Mask{p[0], p[1], p[2], p[3]}, nil
}

// Filter is the ffmpeg drawbox that blacks the mask out.
func (m Mask) Filter() string {
	return fmt.Sprintf("drawbox=x=%s:y=%s:w=%s:h=%s:color=black:t=fill", m.X, m.Y, m.W, m.H)
}
