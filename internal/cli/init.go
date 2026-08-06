package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/layer"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/repo"
	"github.com/coflounder/ilk/internal/targets"
	"github.com/coflounder/ilk/internal/version"
	"github.com/spf13/cobra"
)

// profiles are named sets of layers. They give the one-command, cookiecutter-
// shaped experience without putting anything inside a taxonomy: a profile is a
// one-shot expansion, and nothing you adopt through one is trapped in it.
var profiles = map[string][]string{
	"minimal":  {"ilk/toolkit", "ilk/record"},
	"standard": {"ilk/toolkit", "ilk/record", "ilk/quality-gates"},
}

func profileNames() []string {
	names := make([]string, 0, len(profiles))
	for k := range profiles {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func newInitCmd() *cobra.Command {
	var (
		profile    string
		agents     []string
		testCmd    string
		lintCmd    string
		buildCmd   string
		yes        bool
		noApply    bool
		noBaseline bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set this repository up to work well with coding agents",
		Long: `Create .ilk/, adopt a starting set of layers, and apply them.

With no flags this gives you a project record (what is true, what is intended,
what happened, and an ungoverned scratchpad), instructions in AGENTS.md, one
validator, a session brief, and a pre-commit hook — with no network access and
nothing to choose.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := repo.FindOrInit(workdir())
			if err != nil {
				return err
			}

			if _, err := os.Stat(config.Path(r.Root)); err == nil {
				return fmt.Errorf("%s already exists — this repository is already set up; use `ilk adopt` to add a layer", config.Path(r.Root))
			}

			layers, ok := profiles[profile]
			if !ok {
				return fmt.Errorf("unknown profile %q — one of: %s", profile, strings.Join(profileNames(), ", "))
			}

			cfg := config.Default()
			if len(agents) > 0 {
				cfg.Targets = nil
				for _, a := range agents {
					if a == "none" {
						continue
					}
					if _, err := targets.Get(a); err != nil {
						return err
					}
					cfg.AddTarget(a)
				}
			}
			for name, value := range map[string]string{
				"test.command":  testCmd,
				"lint.command":  lintCmd,
				"build.command": buildCmd,
			} {
				if value != "" {
					cfg.Capabilities[name] = value
				}
			}

			cache := r.IlkPath("cache")
			for _, id := range layers {
				loaded, err := layer.Resolve(id, cache)
				if err != nil {
					return err
				}
				if missing := unsatisfied(loaded, cfg); len(missing) > 0 && id != layers[0] {
					printf("  %s skipping %s — it needs %s (add it later with `ilk adopt %s`)\n",
						sty.yellow("note"), loaded.Manifest.ID, strings.Join(missing, ", "), loaded.Manifest.Name())
					continue
				}
				cfg.Adopt(config.LayerRef{ID: loaded.Manifest.ID, Version: loaded.Manifest.Version})
			}

			if err := os.MkdirAll(r.IlkPath(), 0o755); err != nil {
				return err
			}
			if err := cfg.Save(r.Root); err != nil {
				return err
			}

			p, err := engine.NewProject(r, cfg, lock.New(), version.String())
			if err != nil {
				return err
			}
			pl, err := p.Plan(engine.PlanOptions{Prune: true, NoBaseline: noBaseline})
			if err != nil {
				return err
			}

			if flagJSON {
				return emitJSON(planJSON(pl))
			}

			printf("%s in %s\n\n", sty.bold("ilk init"), r.Root)
			printPlan(pl, false)
			printBaselines(p, pl)

			if noApply {
				printf("\n%s\n", sty.dim("Nothing written except .ilk/config.yaml. Run `ilk apply` when ready."))
				return nil
			}
			if !yes && !confirm("\nApply?") {
				printf("%s\n", sty.dim("Nothing applied. .ilk/config.yaml is written; run `ilk apply` when ready."))
				return nil
			}

			if err := p.Apply(pl); err != nil {
				return err
			}

			printf("\n%s %s\n", sty.green("done."), summariseCounts(pl))
			printNextSteps(p)
			return nil
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "minimal", "starting set of layers: "+strings.Join(profileNames(), ", "))
	cmd.Flags().StringSliceVar(&agents, "agents", nil, "coding agents to generate config for (default: claude-code)")
	cmd.Flags().StringVar(&testCmd, "test-command", "", "how this project runs its tests, e.g. \"go test ./...\"")
	cmd.Flags().StringVar(&lintCmd, "lint-command", "", "how this project lints")
	cmd.Flags().StringVar(&buildCmd, "build-command", "", "how this project builds")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without confirming")
	cmd.Flags().BoolVar(&noApply, "no-apply", false, "write .ilk/config.yaml but change nothing else")
	cmd.Flags().BoolVar(&noBaseline, "no-baseline", false, "check files that already exist in the record directories, instead of exempting them")
	return cmd
}

// unsatisfied reports capabilities a layer needs that the config does not supply.
func unsatisfied(l *layer.Loaded, cfg *config.Config) []string {
	var missing []string
	for _, req := range l.Manifest.Requires {
		if _, ok := cfg.Capabilities[req]; !ok {
			missing = append(missing, req)
		}
	}
	return missing
}

func printNextSteps(p *engine.Project) {
	printf("\n%s\n", sty.bold("Next"))
	printf("  %-28s %s\n", "ilk brief", sty.dim("the packet every session should start with"))
	printf("  %-28s %s\n", "ilk check", sty.dim("validate the repository; failures print their fix"))
	printf("  %-28s %s\n", "ilk list --available", sty.dim("layers you can adopt"))
	if len(p.Config.Capabilities) == 0 {
		printf("\n  %s tell ilk how to verify this project so gates can run:\n", sty.dim("tip"))
		printf("  %s\n", sty.dim("  ilk adopt quality-gates --set test.command=\"<your test command>\""))
	}
}

func confirm(prompt string) bool {
	printf("%s [y/N] ", prompt)
	var answer string
	if _, err := fmt.Fscanln(os.Stdin, &answer); err != nil {
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}
