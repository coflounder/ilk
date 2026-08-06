package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/spf13/cobra"
)

// newBaselineCmd exposes the exemption a layer records when it lands on a
// directory that already had files in it.
//
// The exemption exists so that adopting a layer is not hostile to a repository
// with history. It is deliberately visible and deliberately shrinkable: an
// amnesty nobody can see is indistinguishable from a check that does not work.
func newBaselineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "baseline",
		Short: "Inspect and shrink the files exempted from a layer's checks",
		Long: `When a layer's directory contract lands on a directory that already exists
and already has files, those files are recorded as its baseline and exempted from
that layer's checks. A layer governs what happens next, not what came before.

The exemption is a ratchet. Conform a file to the layer's rules, clear it from the
baseline, and it is held to them from then on.`,
	}
	cmd.AddCommand(newBaselineListCmd(), newBaselineClearCmd())
	return cmd
}

type baselineRow struct {
	Layer string   `json:"layer"`
	Paths []string `json:"paths"`
}

func newBaselineListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show which files are exempt, and from which layer's checks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := project()
			if err != nil {
				return err
			}
			byLayer := p.BaselineByLayer()

			rows := make([]baselineRow, 0, len(byLayer))
			for id, paths := range byLayer {
				sort.Strings(paths)
				rows = append(rows, baselineRow{Layer: id, Paths: paths})
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].Layer < rows[j].Layer })

			if flagJSON {
				return emitJSON(rows)
			}
			if len(rows) == 0 {
				println(sty.dim("Nothing is exempt. Every file in every governed directory is checked."))
				return nil
			}
			for _, r := range rows {
				printf("%s %s\n", sty.bold(r.Layer), sty.dim(fmt.Sprintf("(%d files predate this layer)", len(r.Paths))))
				for _, path := range r.Paths {
					printf("  %s\n", path)
				}
				printf("\n")
			}
			printf("%s\n", sty.dim("These are exempt from their layer's checks. Conform one, then run"))
			printf("%s\n", sty.dim("`ilk baseline clear <path>` to hold it to the rules from now on."))
			return nil
		},
	}
}

func newBaselineClearCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "clear [path...]",
		Short: "Stop exempting files, so their layer's checks apply to them",
		Long: `Remove paths from the baseline. Their layer's checks apply from then on, so
run ` + "`ilk check`" + ` afterwards — if the file was never conformed, this is where
you find out what it needs.

With --all, clears everything. That is the "we are ready to be held to this"
button, and it will usually produce work.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !all {
				return fmt.Errorf("name the paths to clear, or pass --all to clear every exemption")
			}
			p, err := project()
			if err != nil {
				return err
			}

			cleared := p.ClearBaseline(args)
			if len(cleared) == 0 {
				if flagJSON {
					return emitJSON(map[string]any{"cleared": []string{}})
				}
				println(sty.dim("Nothing to clear — none of those paths were exempt. `ilk baseline list` shows what is."))
				return nil
			}
			if err := p.Lock.Save(p.Repo.Root); err != nil {
				return err
			}

			if flagJSON {
				return emitJSON(map[string]any{"cleared": cleared})
			}
			printf("%s %d path(s) are now checked:\n", sty.green("cleared."), len(cleared))
			for _, path := range cleared {
				printf("  %s\n", path)
			}
			printf("\n%s\n", sty.dim("Run `ilk check` to see what they need."))
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "clear every exemption in the repository")
	return cmd
}

// printBaselines renders the baseline a plan is about to record. It belongs in
// the plan output because it is a decision being made on the user's behalf, and
// silently deciding it would be exactly the kind of surprise ilk exists to avoid.
func printBaselines(p *engine.Project, pl *engine.Plan) {
	fresh := map[string][]string{}
	for id, paths := range pl.Baselines {
		if entry, ok := p.Lock.Layer(id); ok && len(entry.Baseline) > 0 {
			continue // already recorded on a previous run
		}
		fresh[id] = paths
	}
	if len(fresh) == 0 {
		return
	}

	ids := make([]string, 0, len(fresh))
	for id := range fresh {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	printf("\n")
	for _, id := range ids {
		paths := fresh[id]
		printf("  %s %s\n", sty.dim("·"), fmt.Sprintf("%d existing file(s) will be exempt from %s's checks", len(paths), id))
		printf("      %s\n", sty.dim(summarisePaths(paths)))
	}
	printf("      %s\n", sty.dim("They predate the layer, so it does not judge them. `ilk baseline` to review."))
}

// summarisePaths lists a few paths and counts the rest, so a directory with two
// hundred files does not bury the plan.
func summarisePaths(paths []string) string {
	const show = 4
	if len(paths) <= show {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(paths[:show], ", "), len(paths)-show)
}
