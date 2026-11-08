package main

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/tjagithub/rosebud/internal/langid"
	"github.com/tjagithub/rosebud/internal/scenes"
	"github.com/tjagithub/rosebud/internal/subs"
)

// minLoud is the loudness below which a window is taken for silence.
const minLoud = 0.003

// langCmd prints the language a film speaks. newDet builds the Detector for a model.
func langCmd(newDet func(model string) langid.Detector) *cobra.Command {
	var model string
	var windows int
	var verbose bool
	var minTop float64
	var win time.Duration
	cmd := &cobra.Command{Use: "lang [flags] FILE", Short: "Tell which language a film speaks", Args: oneFile, RunE: func(c *cobra.Command, args []string) error {
		ctx := c.Context()
		file, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		if _, err := os.Stat(file); err != nil {
			return err
		}
		if windows <= 0 || win <= 0 {
			return fmt.Errorf("usage: %s", c.UseLine())
		}
		var trace io.Writer
		if verbose {
			trace = c.ErrOrStderr()
		}
		r, err := detectLanguage(ctx, newDet(model), file, windows, win, minTop, trace)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(c.OutOrStdout(), "language: %s (score %.2f, %d windows)\n", r.Language, r.Score, r.Windows)
		return err
	}}
	cmd.Flags().Float64Var(&minTop, "min-confidence", 0.7, "a stretch votes only if its best guess is at least this likely")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "show each window's loudness and what the model made of it")
	cmd.Flags().StringVar(&model, "model", "openai/whisper-tiny", "Whisper model that tells the language")
	cmd.Flags().IntVar(&windows, "windows", 48, "how many stretches of audio to listen to")
	cmd.Flags().DurationVar(&win, "window", 10*time.Second, "length of each stretch")
	return cmd
}

// whisperLang is the real detector: Whisper through uv.
func whisperLang(model string) langid.Detector {
	return langid.Whisper{
		Exec: langid.Exec(scenes.NewExec()), Model: model,
		Load: 10 * time.Minute, PerWindow: time.Minute,
	}
}

// tracing prints the two likeliest languages of each window a Detector scores.
type tracing struct {
	langid.Detector
	w io.Writer
}

func (t tracing) Windows(ctx context.Context, wavs []string) ([]langid.Probs, error) {
	out, err := t.Detector.Windows(ctx, wavs)
	for i, p := range out {
		top := slices.SortedFunc(maps.Keys(p), func(a, b string) int { return cmp.Compare(p[b], p[a]) })
		line := filepath.Base(wavs[i]) + ":"
		for _, l := range top[:min(2, len(top))] {
			line += fmt.Sprintf(" %s %.2f", l, p[l])
		}
		_, _ = fmt.Fprintln(t.w, line)
	}
	return out, err
}

// detectLanguage listens to windows stretches of file and names the language.
// A non-nil trace gets each window's loudness and the model's top guesses.
func detectLanguage(ctx context.Context, det langid.Detector, file string, windows int, win time.Duration, minTop float64, trace io.Writer) (langid.Result, error) {
	dir, err := os.MkdirTemp("", "rose-lang-")
	if err != nil {
		return langid.Result{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	ex := subs.Exec(scenes.NewExec())
	total, err := subs.Duration(ctx, ex, file)
	if err != nil {
		return langid.Result{}, err
	}
	cutter := subs.FFmpegCutter(ex, file, dir)
	cut := func(ctx context.Context, at time.Duration) (string, error) {
		wav, err := cutter(ctx, subs.Span{Start: at, End: min(at+win, total)})
		if err != nil {
			return "", err
		}
		rms, err := langid.RMS(wav)
		if trace != nil {
			_, _ = fmt.Fprintf(trace, "%8v rms %.4f %s\n", at.Round(time.Second), rms, filepath.Base(wav))
		}
		if err != nil || rms < minLoud {
			return "", fmt.Errorf("window at %v is silent", at)
		}
		return wav, nil
	}
	if trace != nil {
		det = tracing{det, trace}
	}
	return langid.Detect(ctx, det, cut, langid.Spread(total, windows, win), minTop)
}

// subsDetector builds the language detector that `subs --lang auto` uses.
var subsDetector = whisperLang

// minAutoScore is how sure auto detection must be to pick a language alone.
const minAutoScore = 0.6

// autoLanguage listens to file and returns its language, or says to pass --lang.
func autoLanguage(ctx context.Context, file string, w io.Writer) (string, error) {
	r, err := detectLanguage(ctx, subsDetector("openai/whisper-tiny"), file, 48, 10*time.Second, 0.7, nil)
	if err != nil {
		return "", fmt.Errorf("could not tell the spoken language (%w): pass --lang", err)
	}
	if err := sureEnough(r); err != nil {
		return "", err
	}
	_, _ = fmt.Fprintf(w, "language: %s (score %.2f, %d windows)\n", r.Language, r.Score, r.Windows)
	return r.Language, nil
}

// sureEnough fails for a result too weak to act on alone.
func sureEnough(r langid.Result) error {
	if r.Score < minAutoScore {
		return fmt.Errorf("the audio is mixed or unclear (best guess %s, score %.2f): pass --lang", r.Language, r.Score)
	}
	return nil
}
