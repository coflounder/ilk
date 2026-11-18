package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/layer"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/coflounder/ilk/internal/registry"
	"github.com/spf13/cobra"
)

// expandBundle resolves `@name` to its members' references. A variable so tests
// can supply bundles whose members need no network.
var expandBundle = registry.Expand

// pending is a layer about to be added.
type pending struct {
	ref    string
	loaded *layer.Loaded
}

func newAddCmd() *cobra.Command {
	var (
		set        []string
		vars       []string
		yes        bool
		onConflict string
		allowExec  bool
		dryRun     bool
		noBaseline bool
		replace    bool
	)

	cmd := &cobra.Command{
		Use:   "add <layer>... | @<bundle>",
		Short: "Add layers to this repository",
		Long: `Add one or more layers and reconcile the repository to them.

A layer reference is a built-in name (` + "`record`" + `), a path (` + "`./layers/mine`" + `),
or a git source (` + "`gh:owner/repo@v1`" + `). A bundle (` + "`@plans`" + `) adds each of its
layers in one plan; ` + "`ilk search`" + ` lists the bundles. You see the whole change
before anything is written, and ` + "`ilk rm`" + ` removes exactly what was added.

A repository has one layer for each job: one provider of each capability or role,
one owner of each skill, check and name. Adding a layer that does a job one
already here does is refused; --replace swaps the old layer out in the same plan.`,
		Example: `  ilk add gates --set test.command="go test ./..."
  ilk add @plans
  ilk add blueprint --dry-run
  ilk add gh:acme/llm-wiki --replace`,
		Args: requireArgs(1, "ilk add <layer>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := conflictOptions(onConflict)
			if err != nil {
				return err
			}
			p, err := project()
			if err != nil {
				return err
			}

			// Expand bundles. A member that is already here is skipped rather
			// than refused, so adding a bundle completes a partial adoption.
			type wanted struct {
				ref        string
				fromBundle string
			}
			var refs []wanted
			for _, a := range args {
				if !registry.IsBundleRef(a) {
					refs = append(refs, wanted{ref: a})
					continue
				}
				members, err := expandBundle(a)
				if err != nil {
					return usageError(err)
				}
				for _, m := range members {
					refs = append(refs, wanted{ref: m, fromBundle: a})
				}
			}
			if len(vars) > 0 && len(refs) > 1 {
				return usageError(fmt.Errorf("--var sets one layer's variables, and this adds %d layers — add that layer on its own to set them", len(refs)))
			}

			var adding []pending
			seen := map[string]bool{}
			var skipped []string
			for _, w := range refs {
				loaded, err := resolveLayer(w.ref, p.Repo.IlkPath("cache"))
				if err != nil {
					return err
				}
				m := loaded.Manifest
				if seen[m.ID] {
					continue
				}
				seen[m.ID] = true
				if _, already := p.Config.Layer(m.ID); already {
					if w.fromBundle != "" {
						skipped = append(skipped, m.ID)
						continue
					}
					return fmt.Errorf("%s is already here — use `ilk upgrade %s` to move it to a newer version, or `ilk rm %s` first", m.ID, m.Name(), m.Name())
				}
				adding = append(adding, pending{ref: w.ref, loaded: loaded})
			}
			if len(adding) == 0 {
				printf("%s\n", sty.dim("Every layer in that bundle is already here. Nothing to add."))
				return nil
			}

			// Executable content from a source ilk did not ship is the one thing
			// that needs consent rather than a plan preview.
			if !allowExec {
				var needs []string
				for _, a := range adding {
					if a.loaded.Manifest.NeedsExec() && a.loaded.Source != "builtin" {
						needs = append(needs, fmt.Sprintf("%s (from %s)", a.loaded.Manifest.ID, a.loaded.Source))
					}
				}
				if len(needs) > 0 {
					return &cliError{
						Exit:    exitFailed,
						Code:    "exec_consent_required",
						Message: "these run commands (scripts, checks or subcommands): " + strings.Join(needs, ", "),
						Fix:     "review them with `ilk info <ref>`, then re-run with --allow-exec to consent",
					}
				}
			}

			// One layer per job. Clashes among the new layers cannot be fixed
			// by replacing anything, so they are refused outright; a clash with a
			// layer already here is refused unless --replace says to swap it out.
			var incoming []*manifest.Layer
			newIDs := map[string]bool{}
			for _, a := range adding {
				incoming = append(incoming, a.loaded.Manifest)
				newIDs[a.loaded.Manifest.ID] = true
			}
			if clashes := engine.FindClashes(incoming); len(clashes) > 0 {
				ce := &engine.ClashError{Clashes: clashes}
				return &cliError{Exit: exitFailed, Code: "layer_conflict", Message: ce.Error(), Fix: "add them one at a time and choose which you want"}
			}
			all := append([]*manifest.Layer{}, incoming...)
			for _, l := range p.Layers {
				all = append(all, l.Loaded.Manifest)
			}
			var against []engine.Clash
			displaced := map[string]bool{}
			var displacedOrder []string
			for _, c := range engine.FindClashes(all) {
				involvesNew := false
				for _, id := range c.Layers {
					if newIDs[id] {
						involvesNew = true
					}
				}
				if !involvesNew {
					continue
				}
				against = append(against, c)
				for _, id := range c.Layers {
					if !newIDs[id] && !displaced[id] {
						displaced[id] = true
						displacedOrder = append(displacedOrder, id)
					}
				}
			}
			if len(against) > 0 && !replace {
				ce := &engine.ClashError{Clashes: against}
				return &cliError{
					Exit:    exitFailed,
					Code:    "layer_conflict",
					Message: ce.Error(),
					Fix:     fmt.Sprintf("re-run with --replace to remove %s in the same plan, or keep what is here", strings.Join(displacedOrder, ", ")),
				}
			}
			for _, id := range displacedOrder {
				p.Config.Remove(id)
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

			for _, a := range adding {
				var layerVars map[string]string
				if len(adding) == 1 {
					layerVars = chosen
				}
				if _, err := a.loaded.ResolveVars(layerVars); err != nil {
					return err
				}
				m := a.loaded.Manifest
				p.Config.Set(config.LayerRef{
					ID:        m.ID,
					Version:   m.Version,
					Source:    sourceFor(a.ref, m.ID),
					Vars:      layerVars,
					AllowExec: allowExec,
				})
			}

			next, err := engine.NewProject(p.Repo, p.Config, p.Lock, p.Version)
			if err != nil {
				return err
			}
			opts.Prune, opts.NoBaseline = true, noBaseline
			pl, err := next.Plan(opts)
			if err != nil {
				return err
			}

			if flagJSON {
				if !dryRun {
					if err := p.Config.Save(p.Repo.Root); err != nil {
						return err
					}
					if err := next.Apply(pl); err != nil {
						return err
					}
				}
				return emitJSON(planJSON(pl))
			}

			for _, a := range adding {
				m := a.loaded.Manifest
				printf("%s %s %s\n", sty.bold("add"), m.ID, sty.dim(m.Version))
				printf("  %s\n", sty.dim(m.Summary))
				printf("  %s %s\n", sty.dim("source:"), sty.dim(a.loaded.Source))
			}
			for _, id := range displacedOrder {
				printf("%s %s %s\n", sty.bold("remove"), id, sty.dim("(replaced)"))
			}
			for _, id := range skipped {
				printf("%s\n", sty.dim("skip "+id+" (already here)"))
			}
			printf("\n")
			printPlan(pl, false)
			printBaselines(next, pl)

			if dryRun {
				printf("\n%s\n", sty.dim("Nothing written. Re-run without --dry-run to add it."))
				return nil
			}
			if ok, err := approve(yes, "\nApply?"); !ok {
				if err != nil {
					return err
				}
				println(sty.dim("Nothing applied."))
				return nil
			}
			if err := p.Config.Save(p.Repo.Root); err != nil {
				return err
			}
			if err := next.Apply(pl); err != nil {
				return err
			}
			printf("\n%s %s\n", sty.green("added."), summariseCounts(pl))
			for _, a := range adding {
				printLayerAfterAdd(a.loaded.Manifest)
			}
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&vars, "var", nil, "set a layer variable, e.g. --var docs_dir=documentation (one layer only)")
	cmd.Flags().StringSliceVar(&set, "set", nil, "set a repository capability, e.g. --set test.command=\"go test ./...\"")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without confirming")
	addOnConflictFlag(cmd, &onConflict)
	cmd.Flags().BoolVar(&allowExec, "allow-exec", false, "consent to layers that ship scripts or command-based checks")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the plan and change nothing")
	cmd.Flags().BoolVar(&noBaseline, "no-baseline", false, "check files that already exist in this layer's directories, instead of exempting them")
	cmd.Flags().BoolVar(&replace, "replace", false, "remove the layers already here that do the same job")
	return cmd
}

func printLayerAfterAdd(m *manifest.Layer) {
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

func newRmCmd() *cobra.Command {
	var yes, dryRun bool
	var onConflict string
	cmd := &cobra.Command{
		Use:   "rm <layer>",
		Short: "Remove a layer and everything it added",
		Example: `  ilk rm gates
  ilk rm gates --dry-run
  ilk rm gates --yes`,
		Long: `Remove a layer from this repository.

Files ilk fully owns are deleted; fenced blocks are removed from files it only
partly owns; files it merely seeded are left in place, because they are yours by
then. Anything you edited after ilk wrote it is reported and left alone, unless
--on-conflict take-layer says to remove it anyway.`,
		Args: requireArgs(1, "ilk rm <layer>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := conflictOptions(onConflict)
			if err != nil {
				return err
			}
			p, err := project()
			if err != nil {
				return err
			}
			target, ok := p.Layer(args[0])
			if !ok {
				return fmt.Errorf("%q is not in this repository — `ilk list` shows what is", args[0])
			}
			id := target.ID()

			p.Config.Remove(id)
			next, err := engine.NewProject(p.Repo, p.Config, p.Lock, p.Version)
			if err != nil {
				return err
			}
			opts.Prune = true
			pl, err := next.Plan(opts)
			if err != nil {
				return err
			}

			if flagJSON {
				if !dryRun {
					if err := p.Config.Save(p.Repo.Root); err != nil {
						return err
					}
					if err := next.Apply(pl); err != nil {
						return err
					}
				}
				return emitJSON(planJSON(pl))
			}

			printf("%s %s\n\n", sty.bold("rm"), id)
			printPlan(pl, false)
			if dryRun {
				printf("\n%s\n", sty.dim("Nothing removed. Re-run without --dry-run to remove it."))
				return nil
			}
			if ok, err := approve(yes, "\nApply?"); !ok {
				if err != nil {
					return err
				}
				println(sty.dim("Nothing removed."))
				return nil
			}
			if err := p.Config.Save(p.Repo.Root); err != nil {
				return err
			}
			if err := next.Apply(pl); err != nil {
				return err
			}
			printf("\n%s %s\n", sty.green("removed."), summariseCounts(pl))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without confirming")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the plan and change nothing")
	addOnConflictFlag(cmd, &onConflict)
	return cmd
}

func newUpgradeCmd() *cobra.Command {
	var yes, dryRun bool
	var onConflict string
	cmd := &cobra.Command{
		Use:   "upgrade [layer]",
		Short: "Re-resolve layers and apply any changes they bring",
		Example: `  ilk upgrade
  ilk upgrade record --yes
  ilk upgrade --dry-run
  ilk upgrade --on-conflict markers`,
		Long: `Move this repository's layers to whatever their source now resolves to.

A file you have edited since ilk wrote it is merged rather than overwritten: where
your changes and the layer's touch different parts, both survive. Where they
genuinely collide, ilk refuses and says where. --on-conflict chooses otherwise;
` + "`ilk apply --help`" + ` lists the choices.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := conflictOptions(onConflict)
			if err != nil {
				return err
			}
			p, err := project()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				if _, ok := p.Layer(args[0]); !ok {
					return fmt.Errorf("%q is not in this repository — `ilk list` shows what is", args[0])
				}
			}

			// Record the versions the layers now resolve to.
			var rerecorded bool
			for _, l := range p.Layers {
				if len(args) == 1 && l.ID() != args[0] && l.Name() != args[0] {
					continue
				}
				ref := l.Ref
				if ref.Version != l.Loaded.Manifest.Version {
					rerecorded = true
				}
				ref.Version = l.Loaded.Manifest.Version
				p.Config.Set(ref)
			}

			opts.Prune = true
			pl, err := p.Plan(opts)
			if err != nil {
				return err
			}
			if flagJSON {
				if !dryRun {
					if err := p.Config.Save(p.Repo.Root); err != nil {
						return err
					}
					if err := p.Apply(pl); err != nil {
						return err
					}
				}
				return emitJSON(planJSON(pl))
			}
			if dryRun {
				if pl.Empty() && len(pl.Conflicts()) == 0 {
					println(sty.dim("Every layer is already at the version it resolves to. Nothing written."))
					return nil
				}
				printPlan(pl, false)
				printf("\n%s\n", sty.dim("Nothing written. Re-run without --dry-run to upgrade."))
				return nil
			}
			if pl.Empty() && len(pl.Conflicts()) == 0 {
				// A new version whose content happens to be identical still moved.
				// Returning without saving would leave .ilk/config.yaml claiming the
				// old version for ever, and every later upgrade would agree with it.
				if rerecorded {
					if err := p.Config.Save(p.Repo.Root); err != nil {
						return err
					}
					println(sty.dim("Re-resolved. The versions in .ilk/config.yaml now match what the sources give; no files changed."))
					return nil
				}
				println(sty.dim("Everything is already at the version it resolves to."))
				return nil
			}
			printPlan(pl, false)
			if ok, err := approve(yes, "\nApply?"); !ok {
				if err != nil {
					return err
				}
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
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the plan and change nothing")
	addOnConflictFlag(cmd, &onConflict)
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
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the layers in this repository",
		Long: `List the layers this repository has adopted.

To find layers you could add, use ` + "`ilk search`" + `.`,
		Example: `  ilk list
  ilk list --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := project()
			if err != nil {
				return err
			}
			rows := []layerJSON{}
			for _, l := range p.Layers {
				rows = append(rows, layerJSON{
					ID: l.ID(), Version: l.Loaded.Manifest.Version,
					Summary: l.Loaded.Manifest.Summary, Facets: l.Loaded.Manifest.Facets,
					Adopted: true, Requires: l.Loaded.Manifest.Requires,
					Provides: l.Loaded.Manifest.Provides.Names(), Source: l.Loaded.Source,
				})
			}

			sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
			if flagJSON {
				return emitJSON(rows)
			}

			if len(rows) == 0 {
				println(sty.dim("No layers here yet. `ilk search` shows what you can add."))
				return nil
			}
			for _, r := range rows {
				printf("%s%-24s %-8s %s\n", sty.green("✓ "), r.ID, sty.dim(r.Version), r.Summary)
				if len(r.Requires) > 0 {
					printf("    %s %s\n", sty.dim("requires"), sty.dim(strings.Join(r.Requires, ", ")))
				}
			}
			return nil
		},
	}
	return cmd
}

func newInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <layer>",
		Short: "Show what a layer contains before you add it",
		Example: `  ilk info gates
  ilk info gh:owner/repo --json`,
		Args: requireArgs(1, "ilk info <layer>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			cache := ".ilk/cache"
			if p, err := project(); err == nil {
				cache = p.Repo.IlkPath("cache")
			}
			loaded, err := resolveLayer(args[0], cache)
			if err != nil {
				return err
			}
			m := loaded.Manifest

			if flagJSON {
				return emitJSON(map[string]any{
					"id": m.ID, "version": m.Version, "summary": m.Summary,
					"facets": m.Facets, "source": loaded.Source, "digest": loaded.Digest,
					"examples": nonNil(m.Examples),
					"requires": m.Requires, "provides": m.Provides,
					"variables": m.Variables, "files": m.Files, "dirs": m.Dirs,
					"instructions": m.Instructions, "skills": m.Skills,
					"hooks": m.Hooks, "checks": m.Checks, "commands": m.Commands, "mcp": m.MCP,
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

			section("Examples", m.Examples)

			var reqs []string
			for _, r := range m.Requires {
				reqs = append(reqs, r)
			}
			section("Requires", reqs)
			var provides []string
			for _, name := range m.Provides.Names() {
				if v := m.Provides[name]; v != "" {
					provides = append(provides, fmt.Sprintf("%-24s %s", name, sty.dim(v)))
					continue
				}
				provides = append(provides, name)
			}
			section("Provides", provides)

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

			var mcp []string
			for _, s := range m.MCP {
				mcp = append(mcp, fmt.Sprintf("%-20s %s", s.Name, sty.dim(truncate(s.Summary, 60))))
			}
			section("MCP servers", mcp)

			var cmds []string
			for _, c := range m.Commands {
				cmds = append(cmds, fmt.Sprintf("%-36s %s", "ilk "+m.Name()+" "+layerUseLine(c), sty.dim(c.Summary)))
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
				printf("%s this layer ships executable content; adding it needs --allow-exec\n", sty.yellow("note:"))
			}
			return nil
		},
	}
	return cmd
}

// nonNil keeps an empty list a list in JSON, so a caller can range over it
// without first testing for null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
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
