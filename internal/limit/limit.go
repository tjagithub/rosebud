// Package limit bounds a call so a stuck tool cannot hang its caller, and says
// plainly when the time ran out.
package limit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TimeoutError says that What ran out of time after After.
type TimeoutError struct {
	What  string
	After time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("%s took over %v to complete. Stopping early - to increase the time pass `--timeout %v`",
		e.What, e.After, double(e.After))
}

func (e *TimeoutError) Unwrap() error { return context.DeadlineExceeded }

// double is twice d in the syntax --timeout accepts, without zero units.
func double(d time.Duration) string {
	d *= 2
	if d < time.Second {
		return d.String()
	}
	secs := int(d.Round(time.Second) / time.Second)
	var b strings.Builder
	if h := secs / 3600; h > 0 {
		fmt.Fprintf(&b, "%dh", h)
	}
	if m := secs % 3600 / 60; m > 0 {
		fmt.Fprintf(&b, "%dm", m)
	}
	if s := secs % 60; s > 0 {
		fmt.Fprintf(&b, "%ds", s)
	}
	return b.String()
}

// Run calls fn with a context that ends after d, which must be positive. If
// that deadline ends the call, the error is a *TimeoutError naming what ran.
func Run[T any](ctx context.Context, what string, d time.Duration, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if d <= 0 {
		return zero, fmt.Errorf("limit: %s: timeout must be positive, got %v", what, d)
	}
	inner, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	v, err := fn(inner)
	if err != nil && ctx.Err() == nil && errors.Is(inner.Err(), context.DeadlineExceeded) {
		return zero, &TimeoutError{What: what, After: d}
	}
	return v, err
}
