package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/tjagithub/rosebud/internal/plan"
)

// stage is one step of build: a name and a fresh command to run it with.
type stage struct {
	name  string
	needs []string // stages that must not have failed
	any   []string // at least one of these must not have failed
	cmd   func() *cobra.Command
}

// buildStages is the whole stack, in the order it runs.
func buildStages(d deps) []stage {
	tr := d.transcriber
	if tr == nil {
		tr = parakeet
	}
	return []stage{
		{name: "scenes", cmd: scenesCmd},
		{name: "subs", needs: []string{"scenes"}, cmd: func() *cobra.Command { return subsCmd(tr) }},
		{name: "ocr", needs: []string{"scenes"}, cmd: func() *cobra.Command { return ocrCmd(d.reader) }},
		{name: "translate", needs: []string{"scenes"}, any: []string{"subs", "ocr"}, cmd: func() *cobra.Command { return translateCmd(nil) }},
	}
}

// buildCmd runs every stage on one film. Each stage's flags are offered as
// --STAGE.FLAG (for example --scenes.max-len 30s); --bundle is shared.
func buildCmd(stages []stage) *cobra.Command {
	var bundle string
	var skip []string
	var showPlan bool
	names := make([]string, len(stages))
	for i, s := range stages {
		names[i] = s.name
	}
	cmd := &cobra.Command{
		Use:     "build [flags] FILE",
		Aliases: []string{"create", "all"},
		Short:   "Run every stage on a film: " + strings.Join(names, ", "),
		Long: "Run every stage on a film, each after the stages it needs. A stage that fails\nblocks only the stages that need it; the rest still run.\n\n" +
			"Each stage's own flags work here with the stage name in front, such as\n--scenes.max-len 30s or --translate.backend plamo. See `rose STAGE --help`.",
		Args: oneFile,
		RunE: func(c *cobra.Command, args []string) error {
			for _, s := range skip {
				if !slices.Contains(names, s) {
					return fmt.Errorf("--skip: unknown stage %q (stages: %s)", s, strings.Join(names, ", "))
				}
			}
			steps := planSteps(c, stages, skip, bundle, args[0])
			if showPlan {
				return printPlan(c.OutOrStdout(), steps)
			}
			results, err := plan.Run(c.Context(), steps)
			if err != nil {
				return err
			}
			for _, r := range results {
				if r.Status == plan.Skipped || r.Status == plan.Blocked {
					_, _ = fmt.Fprintf(c.ErrOrStderr(), "%s: %s: %s\n", r.Name, r.Status, r.Reason)
				}
			}
			return plan.Err(results)
		},
	}
	cmd.Flags().StringVar(&bundle, "bundle", "", "bundle directory (default: next to FILE, named FILE.film)")
	cmd.Flags().BoolVar(&showPlan, "plan", false, "show the steps and what each needs, and run nothing")
	cmd.Flags().StringSliceVar(&skip, "skip", nil, "stages to leave out, such as ocr,translate")
	_ = cmd.RegisterFlagCompletionFunc("skip", cobra.FixedCompletions(names, cobra.ShellCompDirectiveNoFileComp))
	_ = cmd.MarkFlagDirname("bundle")
	for _, s := range stages {
		s.cmd().Flags().VisitAll(func(f *pflag.Flag) {
			if f.Name == "bundle" || f.Name == "help" {
				return
			}
			cmd.Flags().AddFlag(&pflag.Flag{
				Name: s.name + "." + f.Name, Usage: f.Usage, Value: f.Value,
				DefValue: f.DefValue, NoOptDefVal: f.NoOptDefVal,
			})
		})
	}
	return cmd
}

// planSteps turns the stages not skipped into steps of a plan. A skipped
// stage is left out of its dependents' needs, since it was asked not to run.
func planSteps(c *cobra.Command, stages []stage, skip []string, bundle, file string) []plan.Step {
	kept := func(names []string) []string {
		var out []string
		for _, n := range names {
			if !slices.Contains(skip, n) {
				out = append(out, n)
			}
		}
		return out
	}
	var steps []plan.Step
	for _, s := range stages {
		if slices.Contains(skip, s.name) {
			continue
		}
		steps = append(steps, plan.Step{Name: s.name, Needs: kept(s.needs), Any: kept(s.any), Run: func(ctx context.Context) error {
			a := []string{file}
			if bundle != "" {
				a = append(a, "--bundle="+bundle)
			}
			c.Flags().Visit(func(f *pflag.Flag) {
				if opt, ok := strings.CutPrefix(f.Name, s.name+"."); ok {
					a = append(a, "--"+opt+"="+f.Value.String())
				}
			})
			err := execute(ctx, s.cmd(), a, c.OutOrStdout(), c.ErrOrStderr())
			var sk *skippedError
			if errors.As(err, &sk) {
				return plan.Skip("%s", sk.reason)
			}
			return err
		}})
	}
	return steps
}

// printPlan lists the steps in the order they would run, with what each needs.
func printPlan(w io.Writer, steps []plan.Step) error {
	ordered, err := plan.Order(steps)
	if err != nil {
		return err
	}
	for _, s := range ordered {
		line := s.Name
		if len(s.Needs) > 0 {
			line += "  needs " + strings.Join(s.Needs, ", ")
		}
		if len(s.Any) > 0 {
			line += "  needs one of " + strings.Join(s.Any, ", ")
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}
