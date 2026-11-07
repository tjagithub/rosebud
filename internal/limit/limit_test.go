package limit_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/limit"
)

func hang(ctx context.Context) (int, error) {
	<-ctx.Done()
	return 0, ctx.Err()
}

func TestRunReturnsTheResult(t *testing.T) {
	got, err := limit.Run(t.Context(), "ffmpeg", time.Minute, func(context.Context) (int, error) { return 7, nil })
	if err != nil || got != 7 {
		t.Errorf("got %d, %v", got, err)
	}
}

func TestRunPassesOtherErrorsThrough(t *testing.T) {
	boom := errors.New("boom")
	_, err := limit.Run(t.Context(), "ffmpeg", time.Minute, func(context.Context) (int, error) { return 0, boom })
	var te *limit.TimeoutError
	if !errors.Is(err, boom) || errors.As(err, &te) {
		t.Errorf("err = %v", err)
	}
}

func TestRunTimesOut(t *testing.T) {
	_, err := limit.Run(t.Context(), "Nemotron-Parse", 50*time.Millisecond, hang)
	var te *limit.TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want a TimeoutError", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Error("TimeoutError does not unwrap to DeadlineExceeded")
	}
	if te.What != "Nemotron-Parse" || te.After != 50*time.Millisecond {
		t.Errorf("fields = %+v", te)
	}
	want := "Nemotron-Parse took over 50ms to complete. Stopping early - to increase the time pass `--timeout 100ms`"
	if err.Error() != want {
		t.Errorf("message = %q\nwant      %q", err, want)
	}
}

func TestSuggestionDoublesInGoDurationSyntax(t *testing.T) {
	for in, want := range map[time.Duration]string{
		time.Minute: "2m", 90 * time.Second: "3m", time.Hour: "2h",
		45 * time.Second: "1m30s", 100 * time.Minute: "3h20m", time.Hour + 30*time.Minute: "3h",
	} {
		msg := (&limit.TimeoutError{What: "x", After: in}).Error()
		if !strings.HasSuffix(msg, "`--timeout "+want+"`") {
			t.Errorf("%v: %q, want suggestion %q", in, msg, want)
		}
	}
}

func TestRunLeavesTheCallersOwnEndAlone(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := limit.Run(ctx, "ffmpeg", time.Hour, hang)
	var te *limit.TimeoutError
	if !errors.Is(err, context.Canceled) || errors.As(err, &te) {
		t.Errorf("cancel: err = %v", err)
	}

	short, stop := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer stop()
	_, err = limit.Run(short, "ffmpeg", time.Hour, hang)
	if !errors.Is(err, context.DeadlineExceeded) || errors.As(err, &te) {
		t.Errorf("caller deadline: err = %v", err)
	}
}

func TestRunNeedsAPositiveDuration(t *testing.T) {
	called := false
	for _, d := range []time.Duration{0, -time.Second} {
		_, err := limit.Run(t.Context(), "ffmpeg", d, func(context.Context) (int, error) { called = true; return 0, nil })
		if err == nil {
			t.Errorf("%v: no error", d)
		}
	}
	if called {
		t.Error("ran with no usable timeout")
	}
}
