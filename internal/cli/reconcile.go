package cli

import (
	"fmt"
	"sort"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/spf13/cobra"
)

// actionJSON is the machine-readable shape of a planned operation.
type actionJSON struct {
	Op     string `json:"op"`
	Path   string `json:"path"`
	Region string `json:"region,omitempty"`
	Owner  string `json:"owner"`
	Mode   string `json:"mode,omitempty"`
	Note   string `json:"note,omitempty"`
}

type planJSONDoc struct {
	Changes   []actionJSON `json:"changes"`
	Conflicts []actionJSON `json:"conflicts"`
	Warnings  []string     `json:"warnings,omitempty"`
	Summary   string       `json:"summary"`
}

func planJSON(pl *engine.Plan) planJSONDoc {
	doc := planJSONDoc{Changes: []actionJSON{}, Conflicts: []actionJSON{}, Warnings: pl.Warnings, Summary: summariseCounts(pl)}
	for _, a := range pl.Changes() {
		doc.Changes = append(doc.Changes, toActionJSON(a))
	}
	for _, a := range pl.Conflicts() {
		doc.Conflicts = append(doc.Conflicts, toActionJSON(a))
	}
	return doc
}

func toActionJSON(a engine.Action) actionJSON {
	return actionJSON{
		Op: string(a.Op), Path: a.Path, Region: a.Region,
		Owner: a.Owner, Mode: string(a.Mode), Note: a.Note,
	}
}

func newPlanCmd() *cobra.Command {
	var all, exitCodeFlag bool
	var onConflict string
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Show what `ilk apply` would change, without changing anything",
		Example: `  ilk plan
  ilk plan --exit-code
  ilk plan --on-conflict take-layer`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, err := conflictOptions(onConflict)
			if err != nil {
				return err
			}
			p, err := project()
			if err != nil {
				return err
			}
			opts.Prune = true
			pl, err := p.Plan(opts)
			if err != nil {
				return err
			}
			if exitCodeFlag && !pl.Empty() {
				exitCode = exitPending
			}
			if flagJSON {
				return emitJSON(planJSON(pl))
			}
			printPlan(pl, all)
			if !pl.Empty() {
				printf("\n%s %s\n", sty.dim("would apply:"), summariseCounts(pl))
			}
			return nil
		},
	}
	addOnConflictFlag(cmd, &onConflict)
	cmd.Flags().BoolVar(&all, "all", false, "include artifacts that are already up to date")
	cmd.Flags().BoolVar(&exitCodeFlag, "exit-code", false, "exit 4 when there are changes to apply, so CI can gate on drift")
	return cmd
}

func newApplyCmd() *cobra.Command {
	var yes, dryRun bool
	var onConflict string
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Reconcile the repository with .ilk/config.yaml",
		Example: `  ilk apply
  ilk apply --yes
  ilk apply --dry-run
  ilk apply --on-conflict markers`,
		Long: `Bring the repository in line with its declared layers.

Apply is idempotent: running it when nothing has changed does nothing.

A file you have edited since ilk wrote it is merged, not overwritten: where your
changes and the layer's touch different parts, both survive. Where they genuinely
collide, ilk refuses and names the lines. --on-conflict chooses otherwise:

  merge        merge, and refuse where the edits collide (the default)
  markers      merge, and write collisions into the file as conflict markers
  keep-mine    keep your version and record it as ilk's new baseline
  take-layer   take the layer's version, discarding yours
  refuse       do not merge at all; refuse every edited file`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, err := conflictOptions(onConflict)
			if err != nil {
				return err
			}
			p, err := project()
			if err != nil {
				return err
			}
			opts.Prune = true
			pl, err := p.Plan(opts)
			if err != nil {
				return err
			}
			if dryRun {
				if flagJSON {
					return emitJSON(planJSON(pl))
				}
				if pl.Empty() && len(pl.Conflicts()) == 0 {
					println(sty.dim("Already up to date."))
					return nil
				}
				printPlan(pl, false)
				printf("\n%s\n", sty.dim("Nothing written. Re-run without --dry-run to apply it."))
				return nil
			}
			if pl.Empty() && len(pl.Conflicts()) == 0 {
				// Still apply. Nothing on disk changes, but this is what records
				// the ancestors a later merge needs — a repository set up before
				// merging existed would otherwise never acquire them, and its
				// first upgrade over an edited file would refuse for no visible
				// reason.
				if err := p.Apply(pl); err != nil {
					return err
				}
				if flagJSON {
					return emitJSON(planJSON(pl))
				}
				println(sty.dim("Already up to date."))
				return nil
			}
			if !flagJSON {
				printPlan(pl, false)
				if ok, err := approve(yes || pl.Empty(), "\nApply?"); !ok {
					if err != nil {
						return err
					}
					println(sty.dim("Nothing applied."))
					return nil
				}
			}
			if err := p.Apply(pl); err != nil {
				return err
			}
			if flagJSON {
				return emitJSON(planJSON(pl))
			}
			printf("\n%s %s\n", sty.green("applied."), summariseCounts(pl))
			if n := len(pl.Conflicts()); n > 0 {
				exitCode = 1
				warnf("%s %d artifact(s) skipped — see above.\n", sty.yellow("note:"), n)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without confirming")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the plan and change nothing")
	addOnConflictFlag(cmd, &onConflict)
	return cmd
}

type statusJSONDoc struct {
	Project      string            `json:"project"`
	Root         string            `json:"root"`
	IlkVersion   string            `json:"ilk_version"`
	Agents       []string          `json:"agents"`
	Layers       []statusLayer     `json:"layers"`
	Capabilities map[string]string `json:"capabilities,omitempty"`
	Drift        []actionJSON      `json:"drift"`
	Warnings     []string          `json:"warnings,omitempty"`
	InSync       bool              `json:"in_sync"`
}

type statusLayer struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Source  string `json:"source"`
	Summary string `json:"summary"`
	Files   int    `json:"files"`
}

func newStatusCmd() *cobra.Command {
	var exitCodeFlag bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show this repository's layers and anything that has drifted",
		Example: `  ilk status
  ilk status --exit-code --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := project()
			if err != nil {
				return err
			}
			pl, err := p.Plan(engine.PlanOptions{Prune: true})
			if err != nil {
				return err
			}

			doc := statusJSONDoc{
				Project:      p.Repo.Name(),
				Root:         p.Repo.Root,
				IlkVersion:   p.Version,
				Agents:       p.Config.Agents,
				Capabilities: p.Config.Capabilities,
				Drift:        []actionJSON{},
				Warnings:     pl.Warnings,
				InSync:       pl.Empty(),
			}
			if exitCodeFlag && !pl.Empty() {
				exitCode = exitPending
			}
			counts := map[string]int{}
			for _, e := range p.Lock.Owners {
				counts[e.ID] = len(e.Files)
			}
			for _, l := range p.Layers {
				doc.Layers = append(doc.Layers, statusLayer{
					ID: l.ID(), Version: l.Loaded.Manifest.Version,
					Source: l.Loaded.Source, Summary: l.Loaded.Manifest.Summary,
					Files: counts[l.ID()],
				})
			}
			for _, a := range pl.Changes() {
				doc.Drift = append(doc.Drift, toActionJSON(a))
			}
			for _, a := range pl.Conflicts() {
				doc.Drift = append(doc.Drift, toActionJSON(a))
			}

			if flagJSON {
				return emitJSON(doc)
			}

			printf("%s  %s\n", sty.bold(doc.Project), sty.dim(doc.Root))
			printf("%s\n\n", sty.dim("ilk "+doc.IlkVersion))

			printf("%s\n", sty.bold("Layers"))
			if len(doc.Layers) == 0 {
				printf("  %s\n", sty.dim("none yet — `ilk search` shows what you can add"))
			}
			for _, l := range doc.Layers {
				printf("  %-24s %-8s %s\n", l.ID, sty.dim(l.Version), sty.dim(fmt.Sprintf("%d files", l.Files)))
			}

			printf("\n%s\n", sty.bold("Agents"))
			printf("  %s\n", sty.dim(joinOr(p.Config.Agents, "none configured")))

			if len(p.Config.Capabilities) > 0 {
				printf("\n%s\n", sty.bold("Capabilities"))
				keys := make([]string, 0, len(p.Config.Capabilities))
				for k := range p.Config.Capabilities {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					printf("  %-18s %s\n", k, sty.dim(p.Config.Capabilities[k]))
				}
			}

			printf("\n%s\n", sty.bold("Sync"))
			if doc.InSync && len(pl.Conflicts()) == 0 {
				printf("  %s\n", sty.green("in sync with .ilk/config.yaml"))
			} else {
				printPlan(pl, false)
				printf("\n  %s\n", sty.dim("run `ilk apply` to reconcile"))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&exitCodeFlag, "exit-code", false, "exit 4 when the repository has drifted from its config")
	return cmd
}

func joinOr(items []string, fallback string) string {
	if len(items) == 0 {
		return fallback
	}
	s := ""
	for i, item := range items {
		if i > 0 {
			s += ", "
		}
		s += item
	}
	return s
}
