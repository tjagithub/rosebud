package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// stage is one step of build: a name and a fresh command to run it with.
type stage struct {
	name string
	cmd  func() *cobra.Command
}

// buildStages is the whole stack, in the order it runs.
func buildStages(d deps) []stage {
	return []stage{
		{"scenes", scenesCmd},
		{"ocr", func() *cobra.Command { return ocrCmd(d.reader) }},
		{"translate", func() *cobra.Command { return translateCmd(nil) }},
	}
}

// buildCmd runs every stage on one film. Each stage's flags are offered as
// --STAGE.FLAG (for example --scenes.max-len 30s); --bundle is shared.
func buildCmd(stages []stage) *cobra.Command {
	var bundle string
	var skip []string
	names := make([]string, len(stages))
	for i, s := range stages {
		names[i] = s.name
	}
	cmd := &cobra.Command{
		Use:     "build [flags] FILE",
		Aliases: []string{"create", "all"},
		Short:   "Run every stage on a film: " + strings.Join(names, ", "),
		Long: "Run every stage on a film, in order, stopping at the first failure.\n\n" +
			"Each stage's own flags work here with the stage name in front, such as\n--scenes.max-len 30s or --translate.backend plamo. See `rose STAGE --help`.",
		Args: oneFile,
		RunE: func(c *cobra.Command, args []string) error {
			for _, s := range skip {
				if !slices.Contains(names, s) {
					return fmt.Errorf("--skip: unknown stage %q (stages: %s)", s, strings.Join(names, ", "))
				}
			}
			for _, s := range stages {
				if slices.Contains(skip, s.name) {
					continue
				}
				a := []string{args[0]}
				if bundle != "" {
					a = append(a, "--bundle="+bundle)
				}
				c.Flags().Visit(func(f *pflag.Flag) {
					if opt, ok := strings.CutPrefix(f.Name, s.name+"."); ok {
						a = append(a, "--"+opt+"="+f.Value.String())
					}
				})
				if err := execute(c.Context(), s.cmd(), a, c.OutOrStdout(), c.ErrOrStderr()); err != nil {
					return fmt.Errorf("%s: %w", s.name, err)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&bundle, "bundle", "", "bundle directory (default: next to FILE, named FILE.film)")
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
