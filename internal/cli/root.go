// Package cli is ilk's command surface.
//
// Two rules shape everything here. Every command supports --json, because agents
// are first-class consumers of this tool rather than an afterthought. And every
// error names the fix, for the same reason checks do: the caller is often an
// agent that can repair the problem itself if told precisely enough what it is.
package cli

import (
	"fmt"
	"os"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/version"
	"github.com/spf13/cobra"
)

var (
	flagJSON bool
	flagDir  string
)

// Execute runs the CLI.
func Execute() int {
	root := newRoot()
	if err := root.Execute(); err != nil {
		fail(err)
		return 1
	}
	return exitCode
}

// exitCode lets commands signal failure without aborting output mid-render.
var exitCode int

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "ilk",
		Short: "Compose the process layers an AI-native repository runs on",
		Long: `ilk manages the layers a repository uses to work well with coding agents:
directory contracts, instructions, skills, hooks, gates and checks.

Layers are adopted, upgraded and dropped at any point in a project's life, not
just at creation. Adopting one shows you the whole change before it happens, and
dropping it removes exactly what it added.

Start with ` + "`ilk init`" + `.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.String(),
	}

	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "emit machine-readable JSON")
	root.PersistentFlags().StringVarP(&flagDir, "dir", "C", "", "run as if ilk was started in this directory")

	root.AddCommand(
		newInitCmd(),
		newAdoptCmd(),
		newDropCmd(),
		newUpgradeCmd(),
		newPlanCmd(),
		newApplyCmd(),
		newStatusCmd(),
		newListCmd(),
		newInfoCmd(),
		newCheckCmd(),
		newBriefCmd(),
		newDoctorCmd(),
		newAgentsCmd(),
		newHookCmd(),
		newLayerCmd(),
		newBaselineCmd(),
		newSearchCmd(),
	)

	// Layer-provided subcommands are attached last, so a layer can never shadow a
	// built-in verb.
	attachLayerCommands(root)

	return root
}

// workdir resolves the directory commands operate from.
func workdir() string {
	if flagDir != "" {
		return flagDir
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// project loads the repository, turning the two common setup failures into
// messages that say what to do next.
func project() (*engine.Project, error) {
	p, err := engine.Load(workdir(), version.String())
	if err != nil {
		return nil, err
	}
	return p, nil
}

// requireArgs produces argument errors in the same voice as everything else.
func requireArgs(n int, usage string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return fmt.Errorf("not enough arguments — usage: %s", usage)
		}
		return nil
	}
}
