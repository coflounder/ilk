package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Command groups, in the order `ilk --help` shows them.
const (
	groupStart     = "start"
	groupLayers    = "layers"
	groupVerify    = "verify"
	groupConfigure = "configure"
	groupShare     = "share"
	groupAuthor    = "author"
	groupLayerCmds = "layer-commands"
	groupPlumbing  = "plumbing"
)

// commandGroups assigns every built-in command to a heading. A built-in missing
// from here is a test failure, not an "Additional Commands" line.
var commandGroups = map[string]string{
	"init": groupStart, "status": groupStart, "brief": groupStart,

	"search": groupLayers, "info": groupLayers, "add": groupLayers, "rm": groupLayers,
	"upgrade": groupLayers, "list": groupLayers, "plan": groupLayers, "apply": groupLayers,

	"check": groupVerify, "doctor": groupVerify,

	"config": groupConfigure, "agents": groupConfigure, "baseline": groupConfigure,

	"contribute": groupShare, "mirror": groupShare,

	"layer": groupAuthor,

	"hook": groupPlumbing, "mcp": groupPlumbing, "self": groupPlumbing,
}

func addGroups(root *cobra.Command) {
	root.AddGroup(
		&cobra.Group{ID: groupStart, Title: "GET STARTED"},
		&cobra.Group{ID: groupLayers, Title: "MANAGE LAYERS"},
		&cobra.Group{ID: groupVerify, Title: "VERIFY"},
		&cobra.Group{ID: groupConfigure, Title: "CONFIGURE"},
		&cobra.Group{ID: groupShare, Title: "SHARE AND INTEGRATE"},
		&cobra.Group{ID: groupAuthor, Title: "AUTHOR LAYERS"},
	)
	for _, c := range root.Commands() {
		if g, ok := commandGroups[c.Name()]; ok {
			c.GroupID = g
		}
	}
}

// addPlumbingGroup comes after the layer commands, so plumbing is listed last.
func addPlumbingGroup(root *cobra.Command) {
	root.AddGroup(&cobra.Group{ID: groupPlumbing, Title: "PLUMBING"})
	root.SetHelpCommandGroupID(groupPlumbing)
	root.SetCompletionCommandGroupID(groupPlumbing)
}

// usageTemplate is cobra's default with two changes: help topics are listed
// under their own heading, as `ilk help <topic>`, before the flags rather than
// after them; and grouped commands have no "Additional Commands" fallback.
const usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasHelpSubCommands}}

HELP TOPICS{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  help {{rpad .Name 12}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

// helpTopicAnnotation marks a command that is only text, read with `ilk help`.
const helpTopicAnnotation = "ilk:help-topic"

func newHelpTopic(name, short, long string) *cobra.Command {
	return &cobra.Command{
		Use:         name,
		Short:       short,
		Long:        long,
		Annotations: map[string]string{helpTopicAnnotation: "true"},
	}
}

func newHelpTopics() []*cobra.Command {
	return []*cobra.Command{
		newHelpTopic("exit-codes", "What each exit code means, for scripts and agents", exitCodesTopic()),
		newHelpTopic("environment", "Environment variables ilk reads, and the ones it gives layer scripts", environmentTopic),
		newHelpTopic("json", "The --json output and the error object", jsonTopic()),
		newHelpTopic("layers", "What a layer is, how to name one, and how to configure it", layersTopic),
	}
}

func exitCodesTopic() string {
	return fmt.Sprintf(`Exit codes are part of ilk's interface. Scripts, CI jobs and agents branch on
them, so codes are only ever added, never renumbered.

  %d  ok            the command did what it was asked, or there was nothing to do
  %d  failed        it ran and the work failed: a check, a refused conflict, a
                   provider command that errored
  %d  usage         it was called wrongly and nothing ran: an unknown command,
                   flag, layer or config key, or a bad argument
  %d  confirmation  it needed a yes it could not ask for (stdin is not a terminal
                   and --yes was not given); nothing was changed
  %d  pending       only with --exit-code (ilk plan, ilk status): the repository is
                   not in sync with .ilk/config.yaml

Two commands sit outside the table on purpose:

  ilk hook run turn-end    exits 2 to block, because Claude Code's Stop hook
                           resumes the agent only on 2
  ilk <layer> <command>    passes its script's exit code through unchanged, once
                           ilk has accepted the call: a missing argument, an
                           unknown flag or an unsupported --json is 2 before the
                           script runs

Examples:
  ilk plan --exit-code     # 0 in sync, 4 changes pending, 1 error
  ilk apply                # with no terminal and no --yes: 3, nothing applied`,
		exitOK, exitFailed, exitUsage, exitConfirmationRequired, exitPending)
}

const environmentTopic = `Variables ilk reads:

  NO_COLOR        any value turns colour off
  TERM=dumb       turns colour off
                  (colour is also off whenever stdout is not a terminal)

Variables ilk sets for the commands layers run. Each runs with the repository
root as its working directory and the rest of ilk's environment passed through.

  Layer commands (ilk <layer> <command>):
    ILK_LAYER           the layer's id, e.g. ilk/record
    ILK_REPO_ROOT       the repository root
    ILK_OUTPUT          json when --json was given (only to a command that
                        declares json: true), otherwise text
    ILK_VAR_<NAME>      each of the layer's variables, upper-cased
    ILK_FLAG_<NAME>     each flag the command declares, upper-cased with dashes
                        as underscores: 1 or 0 for a bool, otherwise the value
                        given or the declared default (empty when neither)
    Arguments arrive as positional parameters. ilk's own flags (--json, -C) never
    reach the script.

  Command-based checks (ilk check):
    ILK_LAYER, ILK_VAR_<NAME>
    A check may name variables of its own under requires_env; it is skipped,
    not failed, while any of them is empty. Its exit code decides the result:
    0 pass, 1 fail, anything else error (the check could not run).

  Mirror provider commands (ilk mirror):
    ILK_LAYER, ILK_REPO_ROOT, ILK_VAR_<NAME>
    ILK_MIRROR_ID, ILK_MIRROR_TITLE, ILK_MIRROR_STATUS, ILK_MIRROR_DOC_ID,
    ILK_MIRROR_PATH     the item being created or updated

  Contribution submit commands (ilk contribute submit):
    ILK_LAYER, ILK_REPO_ROOT, ILK_VAR_<NAME>
    ILK_PROPOSAL_PATH, ILK_PROPOSAL_TITLE, ILK_PROPOSAL_BRANCH,
    ILK_PROPOSAL_FROM, ILK_PROPOSAL_SLUG
    ILK_CONTRIB_REPO, ILK_CONTRIB_PATH, ILK_CONTRIB_BASE

  Hook jobs (ilk hook run):
    ILK_HARNESS         the agent adapter delivering the event, from --target

To choose the repository, use -C/--dir; there is no environment variable for it.`

// errorCodes lists every stable error code a command can return. A code used in
// the source but missing here fails a test, so agents can rely on this list.
var errorCodes = []struct{ code, exit, meaning string }{
	{"usage", "2", "a wrong command, flag or argument"},
	{"unknown_layer", "2", "a layer reference that resolves to nothing"},
	{"invalid_on_conflict", "2", "an --on-conflict value that is not merge, markers, keep-mine, take-layer or refuse"},
	{"unknown_config_key", "2", "a config key that is neither a capability nor a layer variable here"},
	{"invalid_config_key", "2", "a key spelled like neither a capability nor <layer>.<variable>"},
	{"json_unsupported", "2", "--json given to a layer command that does not declare `json: true`"},
	{"core_layer", "2", "an add or rm of a core layer (record, toolkit), which every repository has"},
	{"confirmation_required", "3", "a change that needed a yes nobody could give"},
	{"failed", "1", "the work failed; the message says how"},
	{"layer_conflict", "1", "two layers would do the same job"},
	{"exec_consent_required", "1", "a layer that runs commands, added without --allow-exec"},
	{"config_key_unset", "1", "a capability a layer needs that nothing supplies"},
}

func jsonTopic() string {
	var codes strings.Builder
	for _, c := range errorCodes {
		fmt.Fprintf(&codes, "  %-24s %-4s  %s\n", c.code, c.exit, c.meaning)
	}
	return `--json asks any built-in command for machine-readable output.

Stdout carries the result and nothing else: the plan, the list, the document.
Warnings, notes and prompts go to stderr. Commands that change the repository
(add, rm, upgrade, config set) make the change under --json without asking, and
emit the plan they applied. add, rm, upgrade and apply take --dry-run, which emits
the plan and changes nothing.

When a command fails, stdout holds whatever it had already emitted, and stderr
holds one JSON object:

  {"error": {"code": "unknown_layer", "message": "...", "exit": 2, "fix": "..."}}

  code      stable; branch on this, not on the message
  message   what went wrong, for a person
  exit      the process exit code (see ` + "`ilk help exit-codes`" + `)
  fix       what to do about it, when ilk knows; omitted otherwise

Error codes:
  code                     exit  meaning
` + codes.String() + `
A layer's own command takes --json only when its manifest declares ` + "`json: true`" + `;
it then receives ILK_OUTPUT=json and prints JSON itself. Without that declaration,
--json is refused with json_unsupported and exit 2, rather than answering a
caller who asked for JSON with text.`
}

const layersTopic = `A layer is a versioned package of process: directories, instructions, skills,
hooks, checks and commands that ilk projects into a repository and can take back
out again.

Naming a layer:
  record                    a built-in, or a short name from the index
  ilk/record                its full id
  ./layers/mine             a directory on disk
  gh:owner/repo@v1          a git source, optionally pinned
  @plans                    a bundle: several layers that go together

Capabilities are how layers depend on a project rather than on each other. The
gates layer needs test.command, and anything that supplies it satisfies the need.
A check whose capability is missing is skipped, and says so.

Variables are a layer's own settings, with defaults; ` + "`ilk info <layer>`" + ` lists them,
with the layer's examples of itself in use.

Both are set from the command line, and the repository is reconciled in the same
step:

  ilk config set test.command "go test ./..."
  ilk config set gates.ci none
  ilk config list

A layer's checks are named after it: pr-prep.criteria-settled belongs to pr-prep,
and ` + "`ilk check --only pr-prep`" + ` runs all of them. A layer's commands run as
` + "`ilk <layer> <command>`" + ` and take --help like any other.

Writing a layer: ` + "`ilk layer new`" + ` scaffolds one, and the write-a-layer skill and
docs/reference/REF-writing-layers.md describe the manifest. In short:

  commands:
    - name: draft
      summary: Assemble a pull request description from a plan document
      args:
        - {name: document, required: true, usage: the plan document}
      flags:
        - {name: title, type: bool, usage: print only the title line}
      examples:
        - ilk pr-prep draft docs/plans/SPEC-checkout-totals.md --title
      json: false        # true when the script honours ILK_OUTPUT=json
      run: .ilk/bin/draft.sh

  checks:
    - id: pr-prep.evidence-checkable     # <layer>.<name>
      run: .ilk/bin/evidence.sh          # exit 0 pass, 1 fail, else error
      findings: jsonl                    # optional: {"path","line","message"} per line
      fix: Say what to do about it.`
