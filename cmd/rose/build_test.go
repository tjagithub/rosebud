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
	err := runBuild(t, st, "--skip", "scenes", "film.mkv")
	if err == nil || !strings.Contains(err.Error(), "ocr") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v", err)
	}
	if len(log) != 1 || !strings.HasPrefix(log[0], "ocr ") {
		t.Errorf("log = %q; want only ocr, then stop", log)
	}
}

func TestBuildRejectsUnknownSkip(t *testing.T) {
	var log []string
	if err := runBuild(t, []stage{fakeStage("scenes", &log, nil)}, "--skip", "nope", "film.mkv"); err == nil {
		t.Error("want an error for an unknown stage")
	}
}
