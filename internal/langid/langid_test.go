package langid_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/langid"
)

// fake answers by wav name; a name it does not know scores nil.
type fake map[string]langid.Probs

func (f fake) Windows(_ context.Context, wavs []string) ([]langid.Probs, error) {
	out := make([]langid.Probs, len(wavs))
	for i, w := range wavs {
		out[i] = f[w]
	}
	return out, nil
}

func sec(n int) time.Duration { return time.Duration(n) * time.Second }

// cut names a wav after its start; the window at 99 s cannot be cut.
func cut(_ context.Context, at time.Duration) (string, error) {
	if at == sec(99) {
		return "", errors.New("no audio")
	}
	return at.String(), nil
}

func TestDetect_AveragesAndNamesTheWinner(t *testing.T) {
	d := fake{
		"0s":  {"ja": 0.9, "en": 0.1},
		"10s": {"ja": 0.99, "en": 0.01},
		"20s": {"en": 0.65, "ja": 0.35}, // a fade
	}
	r, err := langid.Detect(t.Context(), d, cut, []time.Duration{0, sec(10), sec(20)})
	if err != nil {
		t.Fatal(err)
	}
	if want := (0.9 + 0.99 + 0.35) / 3; r.Language != "ja" || math.Abs(r.Score-want) > 1e-9 || r.Windows != 3 {
		t.Errorf("got %+v, want ja %v over 3", r, want)
	}
}

func TestDetect_SkipsUncutAndUnscoredWindows(t *testing.T) {
	d := fake{"10s": {"fr": 1}}
	r, err := langid.Detect(t.Context(), d, cut, []time.Duration{0, sec(10), sec(99)})
	if err != nil || r.Language != "fr" || r.Windows != 1 || r.Score != 1 {
		t.Errorf("got %+v, %v", r, err)
	}
	if _, err := langid.Detect(t.Context(), fake{}, cut, []time.Duration{0}); !errors.Is(err, langid.ErrNoSpeech) {
		t.Errorf("err = %v, want ErrNoSpeech", err)
	}
}

type broken struct{}

func (broken) Windows(context.Context, []string) ([]langid.Probs, error) {
	return nil, errors.New("model crashed")
}

func TestDetect_ModelFailureIsAnError(t *testing.T) {
	_, err := langid.Detect(t.Context(), broken{}, cut, []time.Duration{0})
	if err == nil || errors.Is(err, langid.ErrNoSpeech) || !strings.Contains(err.Error(), "model crashed") {
		t.Errorf("err = %v", err)
	}
}

func TestLoudWindows_DropsSilence(t *testing.T) {
	rms := map[time.Duration]float64{0: 0.001, sec(10): 0.07, sec(20): 0.05}
	got := langid.LoudWindows(rms, 0.01)
	if len(got) != 2 || got[0] != sec(10) || got[1] != sec(20) {
		t.Errorf("got %v", got)
	}
}
