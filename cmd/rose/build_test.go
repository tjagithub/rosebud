package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// fakeStage records the args it runs with; it has --bundle and one own flag.
func fakeStage(name string, log *[]string, fail error) stage {
	return stage{name: name, cmd: func() *cobra.Command {
		var v, b string
		c := &cobra.Command{Use: name + " FILE", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
			*log = append(*log, name+" "+strings.Join(args, " ")+" b="+b+" v="+v)
			return fail
		}}
		c.Flags().StringVar(&b, "bundle", "", "")
		c.Flags().StringVar(&v, "opt", "dflt", "an option")
		return c
	}}
}

func runBuild(t *testing.T, stages []stage, args ...string) error {
	t.Helper()
	var out bytes.Buffer
	return execute(context.Background(), buildCmd(stages), args, &out, &out)
}

func TestBuildRunsStagesInOrderWithPrefixedFlags(t *testing.T) {
	var log []string
	st := []stage{fakeStage("scenes", &log, nil), fakeStage("ocr", &log, nil)}
	if err := runBuild(t, st, "--bundle", "/b", "--scenes.opt", "x", "film.mkv"); err != nil {
		t.Fatal(err)
	}
	want := []string{"scenes film.mkv b=/b v=x", "ocr film.mkv b=/b v=dflt"}
	if strings.Join(log, "|") != strings.Join(want, "|") {
		t.Errorf("log = %q, want %q", log, want)
	}
}

func TestBuildSkipAndStopOnFailure(t *testing.T) {
	var log []string
	st := []stage{fakeStage("scenes", &log, nil), fakeStage("ocr", &log, errors.New("boom")), fakeStage("translate", &log, nil)}
	st[2].needs = []string{"ocr"}
	err := runBuild(t, st, "--skip", "scenes", "film.mkv")
	if err == nil || !strings.Contains(err.Error(), "ocr") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v", err)
	}
	if len(log) != 1 || !strings.HasPrefix(log[0], "ocr ") {
		t.Errorf("log = %q; want only ocr, then translate blocked", log)
	}
}

func TestBuildRejectsUnknownSkip(t *testing.T) {
	var log []string
	if err := runBuild(t, []stage{fakeStage("scenes", &log, nil)}, "--skip", "nope", "film.mkv"); err == nil {
		t.Error("want an error for an unknown stage")
	}
}

func TestBuildSkipsAStageWithNothingToDo(t *testing.T) {
	var log []string
	skipErr := &skippedError{stage: "ocr", reason: "no subtitle track"}
	st := []stage{fakeStage("scenes", &log, nil), fakeStage("ocr", &log, skipErr), fakeStage("translate", &log, nil)}
	var out bytes.Buffer
	err := execute(context.Background(), buildCmd(st), []string{"film.mkv"}, &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 3 {
		t.Errorf("a skipped stage must not stop the rest: %q", log)
	}
	if !strings.Contains(out.String(), "ocr: skipped: no subtitle track") {
		t.Errorf("output = %q", out.String())
	}
}

func TestBuildStagesIncludeSpeechByDefault(t *testing.T) {
	var names []string
	for _, s := range buildStages(deps{}) {
		names = append(names, s.name)
	}
	if got, want := strings.Join(names, ","), "scenes,subs,ocr,translate"; got != want {
		t.Errorf("stages = %s, want %s", got, want)
	}
}

func TestBuildFailureBlocksOnlyDependents(t *testing.T) {
	var log []string
	ocr := fakeStage("ocr", &log, errors.New("boom"))
	ocr.needs = []string{"scenes"}
	subs := fakeStage("subs", &log, nil)
	subs.needs = []string{"scenes"}
	tr := fakeStage("translate", &log, nil)
	tr.needs = []string{"ocr"}
	var out bytes.Buffer
	err := execute(context.Background(), buildCmd([]stage{fakeStage("scenes", &log, nil), ocr, subs, tr}), []string{"film.mkv"}, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "ocr: boom") {
		t.Errorf("err = %v", err)
	}
	if len(log) != 3 || !strings.HasPrefix(log[2], "subs ") {
		t.Errorf("subs does not need ocr and must still run, translate must not: %q", log)
	}
	if !strings.Contains(out.String(), "translate: blocked: needs ocr, which failed") {
		t.Errorf("output = %q", out.String())
	}
}

func TestBuildPlanListsStepsAndRunsNothing(t *testing.T) {
	var log []string
	tr := fakeStage("translate", &log, nil)
	tr.needs, tr.any = []string{"scenes"}, []string{"subs", "ocr"}
	st := []stage{tr, fakeStage("subs", &log, nil), fakeStage("ocr", &log, nil), fakeStage("scenes", &log, nil)}
	st[1].needs, st[2].needs = []string{"scenes"}, []string{"scenes"}
	var out bytes.Buffer
	if err := execute(context.Background(), buildCmd(st), []string{"--plan", "film.mkv"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	want := "scenes\nsubs  needs scenes\nocr  needs scenes\ntranslate  needs scenes  needs one of subs, ocr\n"
	if out.String() != want || len(log) != 0 {
		t.Errorf("plan = %q, ran %q", out.String(), log)
	}
}
