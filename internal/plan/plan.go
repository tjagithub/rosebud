// Package plan runs build steps as a graph: a failed step blocks only the
// steps that need it, and a skipped step (nothing to do) counts as met.
package plan

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Step is one unit of work. It needs every step in Needs, and at least one of
// Any if Any is not empty.
type Step struct {
	Name  string
	Needs []string
	Any   []string
	Run   func(context.Context) error
}

// Status is how a step ended.
type Status string

// The ways a step ends.
const (
	Done    Status = "done"
	Skipped Status = "skipped"
	Failed  Status = "failed"
	Blocked Status = "blocked"
)

// Result is the outcome of one step. Reason says why it was skipped or blocked.
type Result struct {
	Name   string
	Status Status
	Reason string
	Err    error
}

type skipError struct{ reason string }

func (e *skipError) Error() string { return "skipped: " + e.reason }

// Skip is the error of a step that had nothing to do.
func Skip(format string, args ...any) error {
	return &skipError{fmt.Sprintf(format, args...)}
}

// Order is the steps in the order they run: each after what it needs, and
// otherwise in the order given. It refuses unknown needs, repeats and cycles.
func Order(steps []Step) ([]Step, error) {
	index := map[string]bool{}
	for _, s := range steps {
		if index[s.Name] {
			return nil, fmt.Errorf("plan: step %q appears twice", s.Name)
		}
		index[s.Name] = true
	}
	for _, s := range steps {
		for _, n := range append(append([]string{}, s.Needs...), s.Any...) {
			if !index[n] {
				return nil, fmt.Errorf("plan: step %q needs unknown step %q", s.Name, n)
			}
		}
	}
	placed := map[string]bool{}
	var out []Step
	for len(out) < len(steps) {
		progress := false
		for _, s := range steps {
			if placed[s.Name] || !allPlaced(placed, s.Needs) || !allPlaced(placed, s.Any) {
				continue
			}
			placed[s.Name] = true
			out = append(out, s)
			progress = true
			break
		}
		if !progress {
			return nil, errors.New("plan: steps need each other in a cycle")
		}
	}
	return out, nil
}

func allPlaced(placed map[string]bool, names []string) bool {
	for _, n := range names {
		if !placed[n] {
			return false
		}
	}
	return true
}

// Run runs the steps one at a time in Order. A step whose needs failed or
// were blocked is blocked, not run; the others carry on.
func Run(ctx context.Context, steps []Step) ([]Result, error) {
	ordered, err := Order(steps)
	if err != nil {
		return nil, err
	}
	end := map[string]Status{}
	var out []Result
	for _, s := range ordered {
		r := Result{Name: s.Name}
		switch reason := unmet(s, end); {
		case ctx.Err() != nil:
			r.Status, r.Reason = Blocked, "cancelled"
		case reason != "":
			r.Status, r.Reason = Blocked, reason
		default:
			r.Status, r.Reason, r.Err = finish(s.Run(ctx))
		}
		end[s.Name] = r.Status
		out = append(out, r)
	}
	return out, nil
}

func finish(err error) (Status, string, error) {
	var sk *skipError
	switch {
	case err == nil:
		return Done, "", nil
	case errors.As(err, &sk):
		return Skipped, sk.reason, nil
	default:
		return Failed, "", err
	}
}

func bad(s Status) bool { return s == Failed || s == Blocked }

// unmet says why s cannot run, or "" if it can.
func unmet(s Step, end map[string]Status) string {
	for _, n := range s.Needs {
		if bad(end[n]) {
			return fmt.Sprintf("needs %s, which %s", n, end[n])
		}
	}
	if len(s.Any) == 0 {
		return ""
	}
	for _, n := range s.Any {
		if !bad(end[n]) {
			return ""
		}
	}
	return "needs one of " + strings.Join(s.Any, ", ") + ", and none worked"
}

// Err is the failures among results, or nil if none failed. Blocked steps
// are explained by the failure that blocked them.
func Err(rs []Result) error {
	var errs []error
	for _, r := range rs {
		if r.Status == Failed {
			errs = append(errs, fmt.Errorf("%s: %w", r.Name, r.Err))
		}
	}
	return errors.Join(errs...)
}
