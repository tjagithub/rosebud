package plan_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tjagithub/rosebud/internal/plan"
)

func step(name string, log *[]string, err error, needs ...string) plan.Step {
	return plan.Step{Name: name, Needs: needs, Run: func(context.Context) error {
		*log = append(*log, name)
		return err
	}}
}

func status(rs []plan.Result) string {
	var s []string
	for _, r := range rs {
		s = append(s, fmt.Sprintf("%s=%s", r.Name, r.Status))
	}
	return strings.Join(s, " ")
}

func TestRunOrdersByNeedsKeepingListOrderOtherwise(t *testing.T) {
	var log []string
	rs, err := plan.Run(t.Context(), []plan.Step{
		step("c", &log, nil, "b"), step("a", &log, nil), step("b", &log, nil, "a"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(log, ""); got != "abc" {
		t.Errorf("ran %q, want abc", got)
	}
	if status(rs) != "a=done b=done c=done" {
		t.Errorf("results %s", status(rs))
	}
}

func TestAFailureBlocksOnlyItsDependents(t *testing.T) {
	var log []string
	rs, _ := plan.Run(t.Context(), []plan.Step{
		step("scenes", &log, nil), step("ocr", &log, errors.New("boom"), "scenes"),
		step("translate", &log, nil, "ocr"), step("subs", &log, nil, "scenes"),
	})
	if status(rs) != "scenes=done ocr=failed translate=blocked subs=done" {
		t.Errorf("results %s", status(rs))
	}
	if rs[2].Reason != "needs ocr, which failed" {
		t.Errorf("reason %q", rs[2].Reason)
	}
	if err := plan.Err(rs); err == nil || !strings.Contains(err.Error(), "ocr: boom") {
		t.Errorf("err %v", err)
	}
}

func TestSkippedStepsSatisfyDependents(t *testing.T) {
	var log []string
	skip := plan.Skip("no bitmap track")
	rs, _ := plan.Run(t.Context(), []plan.Step{
		step("ocr", &log, skip), step("translate", &log, nil, "ocr"),
	})
	if status(rs) != "ocr=skipped translate=done" || rs[0].Reason != "no bitmap track" {
		t.Errorf("results %s / %q", status(rs), rs[0].Reason)
	}
	if plan.Err(rs) != nil {
		t.Error("skips are no error")
	}
}

func TestAnyOfNeedsOneAlternativeThatDidNotFail(t *testing.T) {
	var log []string
	mk := func(sub, ocr error) []plan.Result {
		log = nil
		tr := step("translate", &log, nil)
		tr.Any = []string{"subs", "ocr"}
		rs, _ := plan.Run(t.Context(), []plan.Step{step("subs", &log, sub), step("ocr", &log, ocr), tr})
		return rs
	}
	if got := status(mk(errors.New("x"), nil)); got != "subs=failed ocr=done translate=done" {
		t.Errorf("one left: %s", got)
	}
	if got := status(mk(errors.New("x"), errors.New("y"))); got != "subs=failed ocr=failed translate=blocked" {
		t.Errorf("none left: %s", got)
	}
}

func TestBadGraphsAreRefusedBeforeAnythingRuns(t *testing.T) {
	var log []string
	for name, steps := range map[string][]plan.Step{
		"unknown": {step("a", &log, nil, "nope")},
		"cycle":   {step("a", &log, nil, "b"), step("b", &log, nil, "a")},
		"twice":   {step("a", &log, nil), step("a", &log, nil)},
	} {
		if _, err := plan.Run(t.Context(), steps); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if len(log) != 0 {
		t.Errorf("ran %v", log)
	}
}

func TestCancelledContextStopsTheRest(t *testing.T) {
	var log []string
	ctx, cancel := context.WithCancel(t.Context())
	a := plan.Step{Name: "a", Run: func(context.Context) error { cancel(); return ctx.Err() }}
	rs, _ := plan.Run(ctx, []plan.Step{a, step("b", &log, nil)})
	if len(log) != 0 || rs[1].Status != plan.Blocked {
		t.Errorf("ran %v, %s", log, status(rs))
	}
}

func TestOrderListsTheRunOrder(t *testing.T) {
	var log []string
	got, err := plan.Order([]plan.Step{step("b", &log, nil, "a"), step("a", &log, nil)})
	if err != nil || got[0].Name != "a" || got[1].Name != "b" {
		t.Errorf("%v %v", got, err)
	}
}
