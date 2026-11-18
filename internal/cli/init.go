package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/builtin"
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
//
// Neither names the core tier. record and toolkit are in every repository
// whatever the profile, so listing them here would only suggest they could be
// left out.
var profiles = map[string][]string{
	"default": builtin.Default,
	"minimal": nil,
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
		dryRun     bool
		noBaseline bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set this repository up to work well with coding agents",
		Example: `  ilk init
  ilk init --test-command "go test ./..." --yes
  ilk init --profile minimal
  ilk init --agents claude-code,codex --dry-run`,
		Long: `Create .ilk/, add a starting set of layers, and apply them.

Every repository gets the core tier, which is always on and cannot be removed:
record (what is true, what is intended, what happened, and an ungoverned
scratchpad) and toolkit (how an agent operates ilk itself). It is implied, so
.ilk/config.yaml does not list it.

The default profile adds gates (tests, lint and build as checks, hooks and CI),
secrets (a credential scan before every commit) and compound-lessons (turning a
repeated mistake into a check). Each is an ordinary layer you can ` + "`ilk rm`" + `.
gates is skipped until the repository says how it tests — pass --test-command.
--profile minimal adds nothing beyond the core tier.

All of it is built into the binary: no network access, nothing to choose.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := repo.FindOrInit(workdir())
			if err != nil {
				return err
			}

			if _, err := os.Stat(config.Path(r.Root)); err == nil {
				return fmt.Errorf("%s already exists — this repository is already set up; use `ilk add` to add a layer", config.Path(r.Root))
			}

			layers, ok := profiles[profile]
			if !ok {
				return fmt.Errorf("unknown profile %q — one of: %s", profile, strings.Join(profileNames(), ", "))
			}

			cfg := config.Default()
			if len(agents) > 0 {
				cfg.Agents = nil
				for _, a := range agents {
					if a == "none" {
						continue
					}
					if _, err := targets.Get(a); err != nil {
						return err
					}
					cfg.AddAgent(a)
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
			// What the core tier provides counts as supplied: compound-lessons
			// needs record.docs, and record is always here.
			provided := map[string]bool{}
			for _, id := range builtin.Core {
				loaded, err := layer.Resolve(id, cache)
				if err != nil {
					return err
				}
				for _, c := range loaded.Manifest.Provides.Names() {
					provided[c] = true
				}
			}
			for _, id := range layers {
				loaded, err := layer.Resolve(id, cache)
				if err != nil {
					return err
				}
				if missing := unsatisfied(loaded, cfg, provided); len(missing) > 0 {
					warnf("  %s skipping %s — it needs %s (add it later with `ilk add %s`)\n",
						sty.yellow("note"), loaded.Manifest.ID, strings.Join(missing, ", "), loaded.Manifest.Name())
					continue
				}
				for _, c := range loaded.Manifest.Provides.Names() {
					provided[c] = true
				}
				cfg.Set(config.LayerRef{ID: loaded.Manifest.ID, Version: loaded.Manifest.Version})
			}

			if !dryRun {
				if err := os.MkdirAll(r.IlkPath(), 0o755); err != nil {
					return err
				}
				if err := cfg.Save(r.Root); err != nil {
					return err
				}
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
				// Like add, rm, upgrade and apply: --json applies, because there is
				// nobody to answer a prompt. Stopping after the config used to leave a
				// repository half set up, exit 0, with `init` refusing to run again.
				if !dryRun {
					if err := p.Apply(pl); err != nil {
						return err
					}
				}
				return emitJSON(planJSON(pl))
			}

			printf("%s in %s\n\n", sty.bold("ilk init"), r.Root)
			printPlan(pl, false)
			printBaselines(p, pl)

			if dryRun {
				printf("\n%s\n", sty.dim("Nothing written. Re-run without --dry-run to set the repository up."))
				return nil
			}
			if ok, err := approve(yes, "\nApply?"); !ok {
				if err != nil {
					// .ilk/config.yaml is already written, so `init --yes` would now
					// refuse. Point at the command that finishes the job.
					return errConfirmationRequired(".ilk/config.yaml is written; run `ilk apply --yes` to apply it")
				}
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

	cmd.Flags().StringVar(&profile, "profile", "default", "starting set of layers: "+strings.Join(profileNames(), ", "))
	cmd.Flags().StringSliceVar(&agents, "agents", nil, "coding agents to generate config for (default: claude-code)")
	cmd.Flags().StringVar(&testCmd, "test-command", "", "how this project runs its tests, e.g. \"go test ./...\"")
	cmd.Flags().StringVar(&lintCmd, "lint-command", "", "how this project lints")
	cmd.Flags().StringVar(&buildCmd, "build-command", "", "how this project builds")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without confirming")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the plan and write nothing, not even .ilk/config.yaml")
	cmd.Flags().BoolVar(&noBaseline, "no-baseline", false, "check files that already exist in the record directories, instead of exempting them")
	return cmd
}

// unsatisfied reports capabilities a layer needs that neither the config nor a
// layer already chosen supplies.
func unsatisfied(l *layer.Loaded, cfg *config.Config, provided map[string]bool) []string {
	var missing []string
	for _, req := range l.Manifest.Requires {
		if _, ok := cfg.Capabilities[req]; !ok && !provided[req] {
			missing = append(missing, req)
		}
	}
	return missing
}

func printNextSteps(p *engine.Project) {
	printf("\n%s\n", sty.bold("Next"))
	printf("  %-28s %s\n", "ilk brief", sty.dim("the packet every session should start with"))
	printf("  %-28s %s\n", "ilk check", sty.dim("validate the repository; failures print their fix"))
	printf("  %-28s %s\n", "ilk search", sty.dim("layers you can add"))
	if len(p.Config.Capabilities) == 0 {
		printf("\n  %s tell ilk how to verify this project so gates can run:\n", sty.dim("tip"))
		printf("  %s\n", sty.dim("  ilk add gates --set test.command=\"<your test command>\""))
	}
}
