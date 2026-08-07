package cli

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/coflounder/ilk/internal/targets"
	"github.com/spf13/cobra"
)

func newAgentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Manage which coding agents ilk generates configuration for",
		Long: `Layers never emit agent-specific files. They declare what an agent should
know and do, and ilk projects that into whatever each agent reads.

Adding an agent here changes only generated files. Nothing a layer provides
depends on it — an agent with no integration at all gets the whole feature set by
running the same ` + "`ilk`" + ` commands a human would.`,
	}
	cmd.AddCommand(newAgentsListCmd(), newAgentsAddCmd(), newAgentsRemoveCmd(), newAgentsSyncCmd())
	return cmd
}

func newAgentsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show configured and available agent targets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			enabled := map[string]bool{}
			if p, err := project(); err == nil {
				for _, t := range p.Config.Targets {
					enabled[t] = true
				}
			}
			type row struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Enabled     bool   `json:"enabled"`
				Always      bool   `json:"always"`
			}
			var rows []row
			for _, name := range targets.Names() {
				t, _ := targets.Get(name)
				always := false
				for _, a := range targets.Always {
					if a == name {
						always = true
					}
				}
				rows = append(rows, row{name, t.Description(), enabled[name] || always, always})
			}
			if flagJSON {
				return emitJSON(rows)
			}
			for _, r := range rows {
				mark := sty.dim("  ")
				switch {
				case r.Always:
					mark = sty.green("✓ ")
				case r.Enabled:
					mark = sty.green("✓ ")
				}
				suffix := ""
				if r.Always {
					suffix = sty.dim("  (always on)")
				}
				printf("%s%-14s %s%s\n", mark, r.Name, sty.dim(r.Description), suffix)
			}
			return nil
		},
	}
}

func newAgentsAddCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "add <target>",
		Short: "Generate configuration for another coding agent",
		Args:  requireArgs(1, "ilk agents add <target>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mutateTargets(args[0], true, yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without confirming")
	return cmd
}

func newAgentsRemoveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <target>",
		Short: "Stop generating configuration for an agent, and remove what was generated",
		Args:  requireArgs(1, "ilk agents remove <target>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mutateTargets(args[0], false, yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without confirming")
	return cmd
}

func mutateTargets(name string, add, yes bool) error {
	p, err := project()
	if err != nil {
		return err
	}
	if _, err := targets.Get(name); err != nil {
		return err
	}
	for _, a := range targets.Always {
		if a == name {
			return fmt.Errorf("%s is always on and cannot be changed", name)
		}
	}

	if add {
		if !p.Config.AddTarget(name) {
			return fmt.Errorf("%s is already configured", name)
		}
	} else if !p.Config.RemoveTarget(name) {
		return fmt.Errorf("%s is not configured", name)
	}

	next, err := engine.NewProject(p.Repo, p.Config, p.Lock, p.Version)
	if err != nil {
		return err
	}
	pl, err := next.Plan(engine.PlanOptions{Prune: true})
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
	printPlan(pl, false)
	if !yes && !pl.Empty() && !confirm("\nApply?") {
		println(sty.dim("Nothing applied."))
		return nil
	}
	if err := p.Config.Save(p.Repo.Root); err != nil {
		return err
	}
	if err := next.Apply(pl); err != nil {
		return err
	}
	printf("\n%s %s\n", sty.green("done."), summariseCounts(pl))
	return nil
}

func newAgentsSyncCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Regenerate every agent projection from the current layers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := project()
			if err != nil {
				return err
			}
			pl, err := p.Plan(engine.PlanOptions{Prune: true})
			if err != nil {
				return err
			}
			if flagJSON {
				if err := p.Apply(pl); err != nil {
					return err
				}
				return emitJSON(planJSON(pl))
			}
			if pl.Empty() {
				println(sty.dim("Every agent projection is already up to date."))
				return nil
			}
			printPlan(pl, false)
			if !yes && !confirm("\nApply?") {
				println(sty.dim("Nothing applied."))
				return nil
			}
			if err := p.Apply(pl); err != nil {
				return err
			}
			printf("\n%s %s\n", sty.green("synced."), summariseCounts(pl))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without confirming")
	return cmd
}

type doctorFinding struct {
	Level   string `json:"level"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check that ilk itself is wired up correctly",
		Long: `Report the gaps between what layers declared and what the environment can
actually deliver: hook events an agent cannot fire, git hooks that another tool
has taken over, missing capabilities, and layers whose source has changed.

Degradation is reported rather than hidden, so nobody assumes a gate is running
when it is not.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := project()
			if err != nil {
				return err
			}
			var findings []doctorFinding
			add := func(level, msg, fix string) {
				findings = append(findings, doctorFinding{level, msg, fix})
			}

			if !p.Repo.IsGit() {
				add("warn", "this is not a git repository, so git hooks cannot enforce anything",
					"run `git init` — agent-native hooks still work, but they depend on the agent cooperating")
			} else if path := gitConfigValue(p.Repo.Root, "core.hooksPath"); path != "" {
				add("warn", fmt.Sprintf("core.hooksPath is set to %q, so ilk's git hooks in .git/hooks will not run", path),
					fmt.Sprintf("chain ilk from your hook manager: add `ilk hook run pre-commit` to %s/pre-commit", path))
			}

			if _, err := exec.LookPath("ilk"); err != nil {
				add("warn", "ilk is not on PATH, so generated hooks will skip rather than run",
					"install ilk so that `ilk` resolves in a plain shell")
			}

			ts, err := targets.Resolve(p.Config.Targets)
			if err != nil {
				return err
			}
			for _, event := range manifest.Events {
				hooks := 0
				for _, l := range p.Layers {
					for _, h := range l.Loaded.Manifest.Hooks {
						if h.Event == event {
							hooks++
						}
					}
				}
				if hooks == 0 {
					continue
				}
				var carriers []string
				for _, t := range ts {
					if t.Supports(event) {
						carriers = append(carriers, t.Name())
					}
				}
				if len(carriers) == 0 {
					add("warn", fmt.Sprintf("%d hook(s) registered for %q, which none of your configured agents can deliver", hooks, event),
						"run them yourself with `ilk hook run "+event+"`, or enable an agent that supports the event")
				} else {
					add("ok", fmt.Sprintf("%s delivered by %s", event, strings.Join(carriers, ", ")), "")
				}
			}

			for id, missing := range p.MissingRequirements() {
				add("error", fmt.Sprintf("%s requires %s, which nothing supplies", id, strings.Join(missing, ", ")),
					fmt.Sprintf("set it: ilk add %s --set %s=\"...\", or add it under `capabilities:` in .ilk/config.yaml", id, missing[0]))
			}

			for _, entry := range p.Lock.Owners {
				l, ok := p.Layer(entry.ID)
				if !ok || entry.Digest == "" {
					continue
				}
				if l.Loaded.Digest != entry.Digest {
					add("warn", fmt.Sprintf("%s has changed since it was applied here", entry.ID),
						"run `ilk upgrade "+l.Name()+"` to see what moved")
				}
			}

			pl, err := p.Plan(engine.PlanOptions{Prune: true})
			if err == nil && !pl.Empty() {
				add("warn", fmt.Sprintf("%d generated artifact(s) are out of date", len(pl.Changes())),
					"run `ilk apply`")
			}

			if flagJSON {
				return emitJSON(findings)
			}

			problems := 0
			for _, f := range findings {
				switch f.Level {
				case "ok":
					printf("%s %s\n", sty.green("✓"), sty.dim(f.Message))
				case "warn":
					problems++
					printf("%s %s\n", sty.yellow("!"), f.Message)
					if f.Fix != "" {
						printf("  %s %s\n", sty.cyan("fix:"), f.Fix)
					}
				case "error":
					problems++
					exitCode = 1
					printf("%s %s\n", sty.red("✗"), f.Message)
					if f.Fix != "" {
						printf("  %s %s\n", sty.cyan("fix:"), f.Fix)
					}
				}
			}
			if problems == 0 {
				printf("\n%s\n", sty.green("Everything is wired up."))
			}
			return nil
		},
	}
}

func gitConfigValue(dir, key string) string {
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
