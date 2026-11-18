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
	"strings"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/version"
	"github.com/spf13/cobra"
)

var (
	flagJSON bool
	flagDir  string
)

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	return run(os.Args[1:])
}

// run executes one invocation. It is separate from Execute so tests can drive
// the whole CLI, flags and exit codes included, without a subprocess.
func run(args []string) int {
	exitCode = exitOK
	// Layer commands are attached before cobra parses anything, from whatever
	// repository -C names. Reading the global flags first is what makes
	// `ilk -C other-repo <layer> <command>` find the other repository's layers.
	flagJSON, flagDir = scanGlobalFlags(args)
	root := newRoot()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		json, _ := scanGlobalFlags(args)
		return report(err, flagJSON || json)
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

Layers are added, upgraded and removed at any point in a project's life, not
just at creation. Adding one shows you the whole change before it happens, and
removing it takes away exactly what it added.

Start with ` + "`ilk init`" + `.`,
		Example: `  ilk init
  ilk add gates --set test.command="go test ./..."
  ilk check`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.String(),
	}

	// Defining a flag resets its variable to the default, which would throw away
	// what run() pre-scanned before the layer commands are attached below.
	preJSON, preDir := flagJSON, flagDir
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "emit machine-readable JSON")
	root.PersistentFlags().StringVarP(&flagDir, "dir", "C", "", "run as if ilk was started in this directory")
	flagJSON, flagDir = preJSON, preDir

	// Commands are listed in the order they are added, which is the order a
	// newcomer meets them, rather than alphabetically.
	cobra.EnableCommandSorting = false
	root.AddCommand(
		newInitCmd(),
		newStatusCmd(),
		newBriefCmd(),

		newSearchCmd(),
		newInfoCmd(),
		newAddCmd(),
		newRmCmd(),
		newUpgradeCmd(),
		newListCmd(),
		newPlanCmd(),
		newApplyCmd(),

		newCheckCmd(),
		newDoctorCmd(),

		newConfigCmd(),
		newAgentsCmd(),
		newBaselineCmd(),

		newContributeCmd(),
		newMirrorCmd(),

		newLayerCmd(),

		newHookCmd(),
		newMCPCmd(),
		newSelfCmd(),
	)
	root.AddCommand(newHelpTopics()...)
	root.SetUsageTemplate(usageTemplate)
	root.SetOut(out)
	root.SetErr(errOut)
	addGroups(root)

	// Layer-provided subcommands are attached last, so a layer can never shadow a
	// built-in verb.
	attachLayerCommands(root)
	addPlumbingGroup(root)
	markUsageErrors(root)

	return root
}

// scanGlobalFlags reads --json and -C/--dir from raw arguments, stopping at "--".
// Cobra parses them too; this exists for the moment before it has, when the layer
// commands of the repository -C names are attached, and for reporting an error
// under --json when parsing itself failed.
func scanGlobalFlags(args []string) (asJSON bool, dir string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return asJSON, dir
		case a == "--json" || a == "--json=true":
			asJSON = true
		case a == "--json=false":
			asJSON = false
		case (a == "-C" || a == "--dir") && i+1 < len(args):
			dir = args[i+1]
			i++
		case strings.HasPrefix(a, "--dir="):
			dir = strings.TrimPrefix(a, "--dir=")
		case strings.HasPrefix(a, "-C") && len(a) > 2:
			dir = strings.TrimPrefix(strings.TrimPrefix(a, "-C"), "=")
		}
	}
	return asJSON, dir
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
