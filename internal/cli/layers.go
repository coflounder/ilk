package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/layer"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/spf13/cobra"
)

func newAdoptCmd() *cobra.Command {
	var (
		set        []string
		vars       []string
		yes        bool
		force      bool
		allowExec  bool
		noApply    bool
		noBaseline bool
	)

	cmd := &cobra.Command{
		Use:   "adopt <layer>",
		Short: "Add a layer to this repository",
		Long: `Adopt a layer and reconcile the repository to it.

A layer reference is a built-in name (` + "`record`" + `), a path (` + "`./layers/mine`" + `),
or a git source (` + "`gh:owner/repo@v1`" + `). You see the whole change before anything
is written, and ` + "`ilk drop`" + ` removes exactly what was added.`,
		Args: requireArgs(1, "ilk adopt <layer>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := project()
			if err != nil {
				return err
			}

			ref := args[0]
			loaded, err := layer.Resolve(ref, p.Repo.IlkPath("cache"))
			if err != nil {
				return err
			}
			m := loaded.Manifest

			if _, already := p.Config.Layer(m.ID); already {
				return fmt.Errorf("%s is already adopted — use `ilk upgrade %s` to move it to a newer version, or `ilk drop %s` first", m.ID, m.Name(), m.Name())
			}

			// Executable content from a source ilk did not ship is the one thing
			// that needs consent rather than a plan preview.
			if m.NeedsExec() && loaded.Source != "builtin" && !allowExec {
				return fmt.Errorf("%s runs commands (scripts, checks or subcommands) and comes from %s\n"+
					"  review it first, then re-run with --allow-exec to consent:\n"+
					"    ilk info %s\n"+
					"    ilk adopt %s --allow-exec", m.ID, loaded.Source, ref, ref)
			}

			chosen, err := parseAssignments(vars)
			if err != nil {
				return err
			}
			caps, err := parseAssignments(set)
			if err != nil {
				return err
			}
			for k, v := range caps {
				p.Config.Capabilities[k] = v
			}

			if _, err := loaded.ResolveVars(chosen); err != nil {
				return err
			}

			p.Config.Adopt(config.LayerRef{
				ID:        m.ID,
				Version:   m.Version,
				Source:    sourceFor(ref, m.ID),
				Vars:      chosen,
				AllowExec: allowExec,
			})

			next, err := engine.NewProject(p.Repo, p.Config, p.Lock, p.Version)
			if err != nil {
				return err
			}
			pl, err := next.Plan(engine.PlanOptions{Force: force, Prune: true, NoBaseline: noBaseline})
			if err != nil {
				return err
			}

			if flagJSON {
				if !noApply {
					if err := p.Config.Save(p.Repo.Root); err != nil {
						return err
					}
					if err := next.Apply(pl); err != nil {
						return err
					}
				}
				return emitJSON(planJSON(pl))
			}

			printf("%s %s %s\n", sty.bold("adopt"), m.ID, sty.dim(m.Version))
			printf("  %s\n", sty.dim(m.Summary))
			printf("  %s %s\n\n", sty.dim("source:"), sty.dim(loaded.Source))
			printPlan(pl, false)
			printBaselines(next, pl)

			if noApply {
				printf("\n%s\n", sty.dim("Nothing written. Re-run without --no-apply to adopt."))
				return nil
			}
			if !yes && !confirm("\nApply?") {
				println(sty.dim("Nothing applied."))
				return nil
			}
			if err := p.Config.Save(p.Repo.Root); err != nil {
				return err
			}
			if err := next.Apply(pl); err != nil {
				return err
			}
			printf("\n%s %s\n", sty.green("adopted."), summariseCounts(pl))
			printLayerAfterAdopt(m)
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&vars, "var", nil, "set a layer variable, e.g. --var docs_dir=documentation")
	cmd.Flags().StringSliceVar(&set, "set", nil, "set a repository capability, e.g. --set test.command=\"go test ./...\"")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without confirming")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite files edited since ilk wrote them")
	cmd.Flags().BoolVar(&allowExec, "allow-exec", false, "consent to a layer that ships scripts or command-based checks")
	cmd.Flags().BoolVar(&noApply, "no-apply", false, "show the plan and change nothing")
	cmd.Flags().BoolVar(&noBaseline, "no-baseline", false, "check files that already exist in this layer's directories, instead of exempting them")
	return cmd
}

func printLayerAfterAdopt(m *manifest.Layer) {
	if len(m.Commands) > 0 {
		printf("\n%s\n", sty.bold("New commands"))
		for _, c := range m.Commands {
			printf("  %-30s %s\n", fmt.Sprintf("ilk %s %s", m.Name(), c.Name), sty.dim(c.Summary))
		}
	}
	if len(m.Checks) > 0 {
		var ids []string
		for _, c := range m.Checks {
			ids = append(ids, c.ID)
		}
		printf("\n%s %s\n", sty.dim("new checks:"), sty.dim(strings.Join(ids, ", ")))
	}
}

// sourceFor records how to re-resolve a layer, omitting the reference when it is
// just the layer's own id (a built-in).
func sourceFor(ref, id string) string {
	if ref == id || ref == strings.TrimPrefix(id, "ilk/") {
		return ""
	}
	return ref
}

func newDropCmd() *cobra.Command {
	var yes, force bool
	cmd := &cobra.Command{
		Use:   "drop <layer>",
		Short: "Remove a layer and everything it added",
		Long: `Remove a layer from this repository.

Files ilk fully owns are deleted; fenced blocks are removed from files it only
partly owns; files it merely seeded are left in place, because they are yours by
then. Anything you edited after ilk wrote it is reported and left alone.`,
		Args: requireArgs(1, "ilk drop <layer>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := project()
			if err != nil {
				return err
			}
			target, ok := p.Layer(args[0])
			if !ok {
				return fmt.Errorf("%q is not adopted here — `ilk list` shows what is", args[0])
			}
			id := target.ID()

			p.Config.Drop(id)
			next, err := engine.NewProject(p.Repo, p.Config, p.Lock, p.Version)
			if err != nil {
				return err
			}
			pl, err := next.Plan(engine.PlanOptions{Force: force, Prune: true})
			if err != nil {
				return err
			}

			if flagJSON {
				if err := p.Config.Save(p.Repo.Root); err != nil {
					return err
				}
				if err := next.Apply(pl); err != nil {
					return err
				}
				return emitJSON(planJSON(pl))
			}

			printf("%s %s\n\n", sty.bold("drop"), id)
			printPlan(pl, false)
			if !yes && !confirm("\nApply?") {
				println(sty.dim("Nothing removed."))
				return nil
			}
			if err := p.Config.Save(p.Repo.Root); err != nil {
				return err
			}
			if err := next.Apply(pl); err != nil {
				return err
			}
			printf("\n%s %s\n", sty.green("dropped."), summariseCounts(pl))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without confirming")
	cmd.Flags().BoolVar(&force, "force", false, "remove files even if they were edited after ilk wrote them")
	return cmd
}

func newUpgradeCmd() *cobra.Command {
	var yes, force, noMerge, markers, accept bool
	cmd := &cobra.Command{
		Use:   "upgrade [layer]",
		Short: "Re-resolve layers and apply any changes they bring",
		Long: `Move adopted layers to whatever their source now resolves to.

A file you have edited since ilk wrote it is merged rather than overwritten: where
your changes and the layer's touch different parts, both survive. Where they
genuinely collide, ilk refuses and says where.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := project()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				if _, ok := p.Layer(args[0]); !ok {
					return fmt.Errorf("%q is not adopted here — `ilk list` shows what is", args[0])
				}
			}

			// Record the versions the layers now resolve to.
			for _, l := range p.Layers {
				if len(args) == 1 && l.ID() != args[0] && l.Name() != args[0] {
					continue
				}
				ref := l.Ref
				ref.Version = l.Loaded.Manifest.Version
				p.Config.Adopt(ref)
			}

			pl, err := p.Plan(engine.PlanOptions{Force: force, Prune: true, NoMerge: noMerge, MergeMarkers: markers, Accept: accept})
			if err != nil {
				return err
			}
			if flagJSON {
				if err := p.Config.Save(p.Repo.Root); err != nil {
					return err
				}
				if err := p.Apply(pl); err != nil {
					return err
				}
				return emitJSON(planJSON(pl))
			}
			if pl.Empty() && len(pl.Conflicts()) == 0 {
				println(sty.dim("Everything is already at the version it resolves to."))
				return nil
			}
			printPlan(pl, false)
			if !yes && !confirm("\nApply?") {
				println(sty.dim("Nothing applied."))
				return nil
			}
			if err := p.Config.Save(p.Repo.Root); err != nil {
				return err
			}
			if err := p.Apply(pl); err != nil {
				return err
			}
			printf("\n%s %s\n", sty.green("upgraded."), summariseCounts(pl))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without confirming")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite files edited since ilk wrote them")
	cmd.Flags().BoolVar(&noMerge, "no-merge", false, "refuse edited files instead of merging into them")
	cmd.Flags().BoolVar(&markers, "merge-markers", false, "write conflict markers into files that cannot merge cleanly")
	cmd.Flags().BoolVar(&accept, "accept", false, "keep what is on disk and record it as ilk's new baseline")
	return cmd
}

type layerJSON struct {
	ID       string            `json:"id"`
	Version  string            `json:"version"`
	Summary  string            `json:"summary"`
	Facets   map[string]string `json:"facets,omitempty"`
	Adopted  bool              `json:"adopted"`
	Requires []string          `json:"requires,omitempty"`
	Provides []string          `json:"provides,omitempty"`
	Source   string            `json:"source,omitempty"`
}

func newListCmd() *cobra.Command {
	var available bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List adopted layers, or everything available",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var rows []layerJSON

			adopted := map[string]bool{}
			if p, err := project(); err == nil {
				for _, l := range p.Layers {
					adopted[l.ID()] = true
					rows = append(rows, layerJSON{
						ID: l.ID(), Version: l.Loaded.Manifest.Version,
						Summary: l.Loaded.Manifest.Summary, Facets: l.Loaded.Manifest.Facets,
						Adopted: true, Requires: l.Loaded.Manifest.Requires,
						Provides: l.Loaded.Manifest.Provides, Source: l.Loaded.Source,
					})
				}
			} else if !available {
				return err
			}

			if available {
				builtins, err := layer.Builtins()
				if err != nil {
					return err
				}
				for _, b := range builtins {
					if adopted[b.Manifest.ID] {
						continue
					}
					rows = append(rows, layerJSON{
						ID: b.Manifest.ID, Version: b.Manifest.Version,
						Summary: b.Manifest.Summary, Facets: b.Manifest.Facets,
						Requires: b.Manifest.Requires, Provides: b.Manifest.Provides,
						Source: "builtin",
					})
				}
			}

			sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
			if flagJSON {
				return emitJSON(rows)
			}

			if len(rows) == 0 {
				println(sty.dim("No layers adopted. `ilk list --available` shows what you can add."))
				return nil
			}
			for _, r := range rows {
				mark := sty.dim("  ")
				if r.Adopted {
					mark = sty.green("✓ ")
				}
				printf("%s%-24s %-8s %s\n", mark, r.ID, sty.dim(r.Version), r.Summary)
				if len(r.Requires) > 0 {
					printf("    %s %s\n", sty.dim("requires"), sty.dim(strings.Join(r.Requires, ", ")))
				}
			}
			if available {
				printf("\n%s\n", sty.dim("adopt one with `ilk adopt <name>`; community layers work too: `ilk adopt gh:owner/repo`"))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&available, "available", false, "include layers that are not adopted yet")
	return cmd
}

func newInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <layer>",
		Short: "Show what a layer contains before you adopt it",
		Args:  requireArgs(1, "ilk info <layer>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cache := ".ilk/cache"
			if p, err := project(); err == nil {
				cache = p.Repo.IlkPath("cache")
			}
			loaded, err := layer.Resolve(args[0], cache)
			if err != nil {
				return err
			}
			m := loaded.Manifest

			if flagJSON {
				return emitJSON(map[string]any{
					"id": m.ID, "version": m.Version, "summary": m.Summary,
					"facets": m.Facets, "source": loaded.Source, "digest": loaded.Digest,
					"requires": m.Requires, "provides": m.Provides,
					"variables": m.Variables, "files": m.Files, "dirs": m.Dirs,
					"instructions": m.Instructions, "skills": m.Skills,
					"hooks": m.Hooks, "checks": m.Checks, "commands": m.Commands,
					"needs_exec": m.NeedsExec(), "instruction_budget": m.Budget(),
				})
			}

			printf("%s %s\n", sty.bold(m.ID), sty.dim(m.Version))
			printf("%s\n", m.Summary)
			printf("%s %s\n", sty.dim("source:"), sty.dim(loaded.Source))
			if len(m.Facets) > 0 {
				var facets []string
				for k, v := range m.Facets {
					facets = append(facets, k+"="+v)
				}
				sort.Strings(facets)
				printf("%s %s\n", sty.dim("facets:"), sty.dim(strings.Join(facets, " ")))
			}

			section := func(title string, lines []string) {
				if len(lines) == 0 {
					return
				}
				printf("\n%s\n", sty.bold(title))
				for _, l := range lines {
					printf("  %s\n", l)
				}
			}

			var reqs []string
			for _, r := range m.Requires {
				reqs = append(reqs, r)
			}
			section("Requires", reqs)
			section("Provides", m.Provides)

			var dirs []string
			for _, d := range m.Dirs {
				dirs = append(dirs, fmt.Sprintf("%-16s %s", d.Path, sty.dim(d.Purpose)))
			}
			section("Directories", dirs)

			var files []string
			for _, f := range m.Files {
				files = append(files, fmt.Sprintf("%-40s %s", f.Dest, sty.dim(string(f.Mode))))
			}
			section("Files", files)

			var skills []string
			for _, s := range m.Skills {
				skills = append(skills, fmt.Sprintf("%-20s %s", s.Name, sty.dim(truncate(s.Description, 60))))
			}
			section("Skills", skills)

			var chk []string
			for _, c := range m.Checks {
				chk = append(chk, fmt.Sprintf("%-28s %s", c.ID, sty.dim(c.Title)))
			}
			section("Checks", chk)

			var hooks []string
			for _, h := range m.Hooks {
				hooks = append(hooks, fmt.Sprintf("%-16s %s", h.Event, sty.dim(h.Run)))
			}
			section("Hooks", hooks)

			var cmds []string
			for _, c := range m.Commands {
				cmds = append(cmds, fmt.Sprintf("%-28s %s", "ilk "+m.Name()+" "+c.Name, sty.dim(c.Summary)))
			}
			section("Commands", cmds)

			var vars []string
			for name, v := range m.Variables {
				vars = append(vars, fmt.Sprintf("%-18s %-14s %s", name, sty.dim(v.Default), sty.dim(v.Description)))
			}
			sort.Strings(vars)
			section("Variables", vars)

			if b := m.Budget(); b > 0 {
				printf("\n%s ~%d tokens of always-on instructions\n", sty.dim("context cost:"), b)
			}
			if m.NeedsExec() {
				printf("%s this layer ships executable content; adopting it needs --allow-exec\n", sty.yellow("note:"))
			}
			return nil
		},
	}
	return cmd
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// parseAssignments turns key=value flags into a map, rejecting the shapes that
// would otherwise fail confusingly later.
func parseAssignments(items []string) (map[string]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for _, item := range items {
		key, value, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("%q is not a key=value assignment", item)
		}
		out[strings.TrimSpace(key)] = value
	}
	return out, nil
}
