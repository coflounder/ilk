package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/coflounder/ilk/internal/contrib"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/spf13/cobra"
)

// newContributeCmd is the way back up.
//
// Every other command in ilk moves practice downhill: a layer is published,
// repositories adopt it, upgrades merge improvements in. Without this one the
// traffic is entirely one-way, and a layer only ever learns what its author already
// knew. The repositories using it in anger know more, and until now had nowhere to
// put it.
func newContributeCmd() *cobra.Command {
	var (
		submit  bool
		yes     bool
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "contribute [layer]",
		Short: "Send what this repository learned about a layer back to its maintainer",
		Long: `Draft a proposal for a layer's maintainer, from evidence ilk already holds.

ilk knows what each layer delivered and what the repository decided it should say
instead, so the diff is recorded rather than guessed at. It also knows the things a
diff cannot carry: a default nobody kept, a check nobody could satisfy, an exemption
that never shrank. All of that is gathered automatically.

What is not gathered is the argument. Whether an edit is a fix everybody needs or a
quirk of one repository is a judgement, and the two sections carrying it are left
marked TODO(you). A proposal is not submitted until they are written — a maintainer
receiving diffs with no case attached learns to ignore the whole channel.

    ilk contribute gh-projects              draft the proposal
    ilk contribute gh-projects --submit     open it upstream once the case is written`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := project()
			if err != nil {
				return err
			}
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			l, err := contrib.Find(p, id)
			if err != nil {
				return err
			}
			prop, err := contrib.Build(p, l)
			if err != nil {
				return err
			}

			if submit {
				return runSubmit(p, l, prop, yes, timeout)
			}

			if flagJSON {
				return emitJSON(prop)
			}
			return runDraft(p, l, prop)
		},
	}
	cmd.Flags().BoolVar(&submit, "submit", false, "open the drafted proposal upstream")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "submit without confirming")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "how long to give the submit command")
	cmd.AddCommand(newContributeStatusCmd())
	return cmd
}

func runDraft(p *engine.Project, l *engine.ResolvedLayer, prop *contrib.Proposal) error {
	printProposal(prop)

	if prop.Empty() {
		println(sty.dim("\nNothing to report: this layer is being used exactly as it shipped."))
		println(sty.dim("That is worth knowing too, but it is not a pull request."))
		return nil
	}

	rel, err := contrib.Draft(p, prop)
	if err != nil {
		return err
	}

	printf("\n%s %s\n", sty.green("drafted"), rel)
	if text, ok := contrib.Guidelines(l); ok {
		printf("\n%s\n", sty.bold("What "+prop.Layer+" asks of a proposal"))
		for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
			printf("  %s\n", sty.dim(line))
		}
	}
	printf("\n%s\n", sty.bold("Next"))
	printf("  %-42s %s\n", "write the two TODO(you) sections", sty.dim("the case, which no tool can make"))
	printf("  %-42s %s\n", "ilk contribute "+prop.Layer+" --submit", sty.dim("open it upstream"))
	return nil
}

func runSubmit(p *engine.Project, l *engine.ResolvedLayer, prop *contrib.Proposal, yes bool, timeout time.Duration) error {
	document, blockers, err := contrib.Ready(p, prop)
	if err != nil {
		return err
	}
	if len(blockers) > 0 {
		printf("%s this proposal is not ready to send.\n\n", sty.red("refused:"))
		for _, b := range blockers {
			printf("  %s %s\n", sty.red("!"), b)
		}
		printf("\n  %s\n", sty.dim("Nothing was sent. A proposal is public and permanent once it is opened."))
		exitCode = 1
		return nil
	}

	target := prop.Contribution.Repo
	if prop.Contribution.Path != "" {
		target += "/" + prop.Contribution.Path
	}
	if !yes && !confirm(fmt.Sprintf("Open a pull request against %s?", target)) {
		println(sty.dim("Nothing sent."))
		return nil
	}

	out, err := contrib.Submit(p, l, prop, document, timeout)
	if err != nil {
		return fmt.Errorf("submitting failed: %w", err)
	}
	printf("\n%s %s\n", sty.green("opened."), out)
	return nil
}

func newContributeStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show, for every layer here, what there is to send back",
		Long: `Survey every layer here at once.

Useful periodically rather than per-layer: contributing is the kind of task that never
becomes urgent, and a repository that has quietly diverged from six layers is the normal
outcome of using them for a year.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := project()
			if err != nil {
				return err
			}
			type row struct {
				layer   string
				edits   int
				signals int
				open    bool
			}
			var rows []row
			var props []*contrib.Proposal
			for _, l := range p.Layers {
				prop, err := contrib.Build(p, l)
				if err != nil {
					return err
				}
				props = append(props, prop)
				rows = append(rows, row{
					layer:   l.ID(),
					edits:   len(prop.Edits),
					signals: len(prop.Signals),
					open:    l.Loaded.Manifest.Contribution != nil,
				})
			}

			if flagJSON {
				return emitJSON(props)
			}

			any := false
			for _, r := range rows {
				if r.edits == 0 && r.signals == 0 {
					continue
				}
				any = true
				what := []string{}
				if r.edits > 0 {
					what = append(what, fmt.Sprintf("%d edit(s)", r.edits))
				}
				if r.signals > 0 {
					what = append(what, fmt.Sprintf("%d signal(s)", r.signals))
				}
				mark := sty.cyan("→")
				note := ""
				if !r.open {
					mark = sty.dim("·")
					note = sty.dim("  (no contribution: block — its maintainer has not opted in)")
				}
				printf("  %s %-26s %s%s\n", mark, r.layer, strings.Join(what, ", "), note)
			}
			if !any {
				println(sty.dim("Every layer here is being used exactly as it shipped."))
				return nil
			}
			printf("\n  %s\n", sty.dim("ilk contribute <layer>  drafts a proposal from this evidence"))
			return nil
		},
	}
}

func printProposal(prop *contrib.Proposal) {
	if len(prop.Edits) > 0 {
		printf("%s\n", sty.bold("Changed after "+prop.Layer+" wrote it"))
		for _, e := range prop.Edits {
			label := e.Path
			if e.Region != "" {
				label += " [" + e.Region + "]"
			}
			marker := sty.cyan("~")
			if e.Accepted {
				marker = sty.green("=")
			}
			printf("  %s %-44s %s\n", marker, label, sty.dim(e.HistoryPhrase()))
			if !e.Portable {
				printf("      %s\n", sty.dim("rendered with this repository's values — evidence, not a patch"))
			}
		}
	}

	if len(prop.Signals) > 0 {
		printf("\n%s\n", sty.bold("Where "+prop.Layer+" is fighting this repository"))
		for _, s := range prop.Signals {
			printf("  %s %-14s %-20s %s\n", sty.yellow("!"), s.Kind, s.Subject, sty.dim(s.Detail))
		}
	}

	if blocking := prop.Blocking(); len(blocking) > 0 {
		printf("\n%s %d thing(s) must not leave this repository.\n", sty.red("blocked:"), len(blocking))
		for _, c := range blocking {
			printf("  %s %s line %d — %s\n", sty.red("!"), c.Path, c.Line, c.Reason)
		}
	}
}
