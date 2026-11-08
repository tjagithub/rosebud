package langid_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/langid"
)

type fake map[time.Duration]langid.Probs

func (f fake) Window(_ context.Context, at time.Duration) (langid.Probs, error) {
	p, ok := f[at]
	if !ok {
		return nil, errors.New("no window")
	}
	return p, nil
}

func sec(n int) time.Duration { return time.Duration(n) * time.Second }

func TestDetect_AveragesAndNamesTheWinner(t *testing.T) {
	d := fake{
		0:       {"ja": 0.9, "en": 0.1},
		sec(10): {"ja": 0.99, "en": 0.01},
		sec(20): {"en": 0.65, "ja": 0.35}, // a fade
	}
	r, err := langid.Detect(t.Context(), d, []time.Duration{0, sec(10), sec(20)})
	if err != nil {
		t.Fatal(err)
	}
	if want := (0.9 + 0.99 + 0.35) / 3; r.Language != "ja" || r.Score < want-1e-9 || r.Score > want+1e-9 || r.Windows != 3 {
		t.Errorf("got %+v, want ja %v over 3", r, want)
	}
}

func TestDetect_SkipsFailedWindowsButNeedsOne(t *testing.T) {
	d := fake{sec(10): {"fr": 1}}
	r, err := langid.Detect(t.Context(), d, []time.Duration{0, sec(10)})
	if err != nil || r.Language != "fr" || r.Windows != 1 || r.Score != 1 {
		t.Errorf("got %+v, %v", r, err)
	}
	if _, err := langid.Detect(t.Context(), fake{}, []time.Duration{0}); !errors.Is(err, langid.ErrNoSpeech) {
		t.Errorf("err = %v, want ErrNoSpeech", err)
	}
}

func TestLoudWindows_DropsSilence(t *testing.T) {
	rms := map[time.Duration]float64{0: 0.001, sec(10): 0.07, sec(20): 0.05}
	got := langid.LoudWindows(rms, 0.01)
	if len(got) != 2 || got[0] != sec(10) || got[1] != sec(20) {
		t.Errorf("got %v", got)
	}
}
