package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/coflounder/ilk/internal/checks"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/spf13/cobra"
)

func newCheckCmd() *cobra.Command {
	var (
		only    []string
		skip    []string
		fix     bool
		list    bool
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate the repository; every failure prints its own fix",
		Long: `Run every check the adopted layers register, plus the ones ilk always runs.

Exits non-zero when anything fails, so it can be wired into hooks and CI. Each
failure prints what is wrong and what to do about it — precisely enough that an
agent can repair it and run the check again without asking anyone.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := project()
			if err != nil {
				return err
			}

			if list {
				ids, err := checks.IDs(p)
				if err != nil {
					return err
				}
				if flagJSON {
					return emitJSON(ids)
				}
				for _, id := range ids {
					println(id)
				}
				return nil
			}

			if fix {
				if err := applyFix(p); err != nil {
					return err
				}
				// Re-load so the run below sees the repaired state.
				if p, err = project(); err != nil {
					return err
				}
			}

			report, err := checks.Run(p, checks.Options{Only: only, Skip: skip, Timeout: timeout})
			if err != nil {
				return err
			}

			if flagJSON {
				if !report.OK() {
					exitCode = 1
				}
				return emitJSON(report)
			}

			printReport(report)
			if !report.OK() {
				exitCode = 1
			}
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&only, "only", nil, "run only these checks (comma-separated ids)")
	cmd.Flags().StringSliceVar(&skip, "skip", nil, "skip these checks")
	cmd.Flags().BoolVar(&fix, "fix", false, "regenerate ilk-managed files before checking, repairing drift")
	cmd.Flags().BoolVar(&list, "list", false, "list available check ids and exit")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "per-check timeout for command-based checks")
	return cmd
}

// applyFix repairs the one class of failure ilk can fix by itself: generated
// content that no longer matches what the layers say it should be.
func applyFix(p *engine.Project) error {
	pl, err := p.Plan(engine.PlanOptions{Prune: true})
	if err != nil {
		return err
	}
	if pl.Empty() {
		return nil
	}
	if err := p.Apply(pl); err != nil {
		return err
	}
	if !flagJSON {
		printf("%s %s\n\n", sty.dim("regenerated:"), summariseCounts(pl))
	}
	return nil
}

func printReport(r *checks.Report) {
	for _, res := range r.Results {
		switch res.Status {
		case checks.StatusPass:
			printf("%s %-28s %s\n", sty.green("✓"), res.ID, sty.dim(res.Title))
		case checks.StatusSkip:
			printf("%s %-28s %s\n", sty.dim("−"), res.ID, sty.dim(res.Reason))
		case checks.StatusError:
			printf("%s %-28s %s\n", sty.yellow("!"), res.ID, sty.yellow("could not run: "+res.Reason))
		case checks.StatusFail:
			printf("%s %-28s %s\n", sty.red("✗"), res.ID, res.Title)
			for _, f := range res.Findings {
				location := f.Path
				if f.Line > 0 {
					location = fmt.Sprintf("%s:%d", f.Path, f.Line)
				}
				if location != "" {
					printf("    %s  %s\n", sty.bold(location), f.Message)
				} else {
					printf("    %s\n", f.Message)
				}
			}
			if res.Output != "" {
				for _, line := range lastLines(res.Output, 12) {
					printf("    %s\n", sty.dim(line))
				}
			}
			if res.Fix != "" {
				printf("    %s %s\n", sty.cyan("fix:"), res.Fix)
			}
		}
	}

	printf("\n")
	var parts []string
	if r.Passed > 0 {
		parts = append(parts, sty.green(fmt.Sprintf("%d passing", r.Passed)))
	}
	if r.Failed > 0 {
		parts = append(parts, sty.red(fmt.Sprintf("%d failing", r.Failed)))
	}
	if r.Errored > 0 {
		parts = append(parts, sty.yellow(fmt.Sprintf("%d could not run", r.Errored)))
	}
	if r.Skipped > 0 {
		parts = append(parts, sty.dim(fmt.Sprintf("%d skipped", r.Skipped)))
	}
	if len(parts) == 0 {
		parts = append(parts, sty.dim("no checks registered — adopt a layer that brings some"))
	}
	println(strings.Join(parts, "  "))
}

// lastLines trims command output to the tail, which is where the failure is.
func lastLines(s string, n int) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return lines
	}
	return append([]string{fmt.Sprintf("… %d earlier lines", len(lines)-n)}, lines[len(lines)-n:]...)
}
