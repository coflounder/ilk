package cli

import (
	"fmt"
	"time"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/coflounder/ilk/internal/mirror"
	"github.com/spf13/cobra"
)

// newMirrorCmd is ilk's surface onto somebody else's system.
//
// It keeps the shape ilk uses everywhere else, because that shape is what makes a
// write to a tracker safe: you see the whole plan before anything executes, and
// applying is a separate deliberate step. A write is either correct or it does
// not happen.
func newMirrorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mirror",
		Short: "Keep the record and an external tracker in agreement",
		Long: `Reconcile record documents with a tracker — a GitHub Project, a Linear team.

The record is the source of truth. This makes the tracker match the markdown, never
the other way round, so the question "which one is right" never has to be asked.

ilk supplies identity, diffing, refusing on ambiguity and the plan-then-apply
discipline; an adopted layer supplies the three commands that know the provider.`,
	}
	cmd.AddCommand(newMirrorPlanCmd(), newMirrorApplyCmd(), newMirrorLinkCmd())
	return cmd
}

func mirrorOptions(link bool, timeout time.Duration) mirror.Options {
	return mirror.Options{Link: link, Timeout: timeout}
}

func newMirrorPlanCmd() *cobra.Command {
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "plan [mirror]",
		Short: "Show what reconciling would change in the tracker, without changing it",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMirror(args, func(ctx mirrorContext) error {
				plan, err := mirror.Build(ctx.project, ctx.layer, ctx.mirror, mirrorOptions(false, timeout))
				if err != nil {
					return err
				}
				if flagJSON {
					if len(plan.Blocked()) > 0 {
						exitCode = 1
					}
					return emitJSON(plan)
				}
				printMirrorPlan(plan)
				if len(plan.Blocked()) > 0 {
					exitCode = 1
				}
				return nil
			})
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "how long to give each provider command")
	return cmd
}

func newMirrorApplyCmd() *cobra.Command {
	var (
		yes     bool
		link    bool
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "apply [mirror]",
		Short: "Make the tracker match the record",
		Long: `Write the planned changes to the tracker.

Documents that could mean more than one tracker item are never applied — the plan
refuses and names the candidates. Everything unambiguous still goes through, so a run
that hits one is not wasted.

Nothing is ever deleted in the tracker. An item with nothing in the record pointing at
it is reported, because deciding it is dead is not ilk's call to make.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMirror(args, func(ctx mirrorContext) error {
				plan, err := mirror.Build(ctx.project, ctx.layer, ctx.mirror, mirrorOptions(link, timeout))
				if err != nil {
					return err
				}

				if plan.Empty() {
					if flagJSON {
						return emitJSON(plan)
					}
					printMirrorPlan(plan)
					println(sty.dim("Nothing to write. The tracker already matches the record."))
					if len(plan.Blocked()) > 0 {
						exitCode = 1
					}
					return nil
				}

				if !flagJSON {
					printMirrorPlan(plan)
					if !yes && !confirm(fmt.Sprintf("\nWrite %d change(s) to %s?", len(plan.Writes()), ctx.mirror.Summary)) {
						println(sty.dim("Nothing written."))
						return nil
					}
				}

				result, err := mirror.Apply(ctx.project, ctx.layer, ctx.mirror, plan, mirrorOptions(link, timeout))
				if err != nil {
					return err
				}
				if flagJSON {
					if len(result.Errors) > 0 || len(plan.Blocked()) > 0 {
						exitCode = 1
					}
					return emitJSON(result)
				}
				printMirrorResult(result, len(plan.Blocked()))
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "write without confirming")
	cmd.Flags().BoolVar(&link, "link", false, "also match unlinked documents to existing items by title")
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "how long to give each provider command")
	return cmd
}

func newMirrorLinkCmd() *cobra.Command {
	var (
		yes     bool
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "link [mirror]",
		Short: "Match documents to items that already exist in the tracker",
		Long: `Adopt a tracker that already has content.

Unlinked documents are matched to existing items by title, and the match is recorded
in the frontmatter key ilk owns. From then on identity is exact and titles no longer
matter.

Where a title could mean more than one item, ilk refuses and names both. A wrong link
is silent and permanent: every later sync writes to the wrong item, and nobody finds
out until somebody reads the tracker and does not recognise it.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMirror(args, func(ctx mirrorContext) error {
				plan, err := mirror.Build(ctx.project, ctx.layer, ctx.mirror, mirrorOptions(true, timeout))
				if err != nil {
					return err
				}

				// Linking records identity; it does not create or update.
				var links []mirror.Action
				for _, a := range plan.Actions {
					if a.Op == mirror.OpLink {
						links = append(links, a)
					}
				}
				linkOnly := &mirror.Plan{Mirror: plan.Mirror, Actions: append(links, plan.Blocked()...)}

				if flagJSON {
					if len(plan.Blocked()) > 0 {
						exitCode = 1
					}
					if len(links) == 0 {
						return emitJSON(linkOnly)
					}
					result, err := mirror.Apply(ctx.project, ctx.layer, ctx.mirror, linkOnly, mirrorOptions(true, timeout))
					if err != nil {
						return err
					}
					return emitJSON(result)
				}

				printMirrorPlan(linkOnly)
				if len(links) == 0 {
					println(sty.dim("Nothing to link."))
					if len(plan.Blocked()) > 0 {
						exitCode = 1
					}
					return nil
				}
				if !yes && !confirm(fmt.Sprintf("\nRecord %d link(s)?", len(links))) {
					println(sty.dim("Nothing linked."))
					return nil
				}
				result, err := mirror.Apply(ctx.project, ctx.layer, ctx.mirror, linkOnly, mirrorOptions(true, timeout))
				if err != nil {
					return err
				}
				printMirrorResult(result, len(plan.Blocked()))
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "record links without confirming")
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "how long to give each provider command")
	return cmd
}

type mirrorContext struct {
	project *engine.Project
	layer   *engine.ResolvedLayer
	mirror  manifest.Mirror
}

func withMirror(args []string, fn func(mirrorContext) error) error {
	p, err := project()
	if err != nil {
		return err
	}
	id := ""
	if len(args) == 1 {
		id = args[0]
	}
	l, m, err := mirror.Find(p, id)
	if err != nil {
		return err
	}
	return fn(mirrorContext{project: p, layer: l, mirror: m})
}

func printMirrorPlan(plan *mirror.Plan) {
	if len(plan.Actions) == 0 {
		println(sty.dim("Nothing to reconcile."))
		return
	}

	for _, a := range plan.Actions {
		switch a.Op {
		case mirror.OpAmbiguous:
			printf("  %s %-10s %s\n", sty.red("!"), "AMBIGUOUS", a.Title)
			printf("      %s\n", sty.dim(a.Path+" — "+a.Detail))
			for _, c := range a.Candidates {
				label := c.ID
				if c.URL != "" {
					label = c.URL
				}
				printf("        %s %s\n", sty.dim("could be"), label)
			}
		case mirror.OpCreate:
			printf("  %s %-10s %s\n", sty.green("+"), "create", a.Title)
			printf("      %s\n", sty.dim(a.Path))
		case mirror.OpUpdate:
			printf("  %s %-10s %s\n", sty.cyan("~"), "update", a.Title)
			for _, c := range a.Changes {
				printf("      %s %s: %s %s %s\n", sty.dim("·"), c.Field,
					sty.dim(quoteOrEmpty(c.Remote)), sty.dim("→"), quoteOrEmpty(c.Record))
			}
		case mirror.OpLink:
			printf("  %s %-10s %s\n", sty.cyan("="), "link", a.Title)
			printf("      %s\n", sty.dim(a.Path+" → "+a.RemoteID))
		case mirror.OpOrphan:
			printf("  %s %-10s %s\n", sty.yellow("?"), "orphan", a.Title)
			printf("      %s\n", sty.dim(a.Path+" — "+a.Detail))
		case mirror.OpUntracked:
			printf("  %s %-10s %s\n", sty.dim("·"), "untracked", a.Title)
			printf("      %s\n", sty.dim(a.Detail))
		}
	}

	if blocked := plan.Blocked(); len(blocked) > 0 {
		printf("\n  %s %d document(s) could mean more than one item in the tracker.\n",
			sty.red("refused:"), len(blocked))
		printf("  %s\n", sty.dim("Rename one side so the titles differ, or record the right id by hand"))
		printf("  %s\n", sty.dim("in the document's frontmatter. Guessing here writes to the wrong item"))
		printf("  %s\n", sty.dim("for ever, and nobody notices until they read the tracker."))
	}
}

func quoteOrEmpty(s string) string {
	if s == "" {
		return sty.dim("(empty)")
	}
	return `"` + s + `"`
}

func printMirrorResult(r *mirror.Result, blocked int) {
	var parts []string
	if r.Created > 0 {
		parts = append(parts, fmt.Sprintf("%d created", r.Created))
	}
	if r.Updated > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", r.Updated))
	}
	if r.Linked > 0 {
		parts = append(parts, fmt.Sprintf("%d linked", r.Linked))
	}
	if len(parts) == 0 {
		parts = append(parts, "nothing written")
	}

	printf("\n%s %s\n", sty.green("done."), joinComma(parts))
	for _, e := range r.Errors {
		printf("%s %s\n", sty.red("failed:"), e)
	}
	if len(r.Errors) > 0 || blocked > 0 {
		exitCode = 1
	}
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
