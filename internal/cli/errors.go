package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/spf13/cobra"
)

// Exit codes. Scripts and agents branch on these, so they are part of the
// interface: add to the list, never renumber it.
//
// Two commands sit outside the table on purpose. `ilk hook run` speaks each
// agent's hook protocol, where 2 means "block". A layer's own subcommand passes
// its script's exit code through unchanged.
const (
	exitOK                   = 0 // the command did what it was asked
	exitFailed               = 1 // it ran, and the work failed: a check, a conflict, an error
	exitUsage                = 2 // it was called wrongly: unknown command or flag, missing argument
	exitConfirmationRequired = 3 // it needed a yes it could not ask for; nothing was changed
	exitPending              = 4 // with --exit-code: the repository is not in sync with its config
)

// cliError is an error that knows its exit code and a stable, machine-readable
// code. Agents branch on Code; people read Message and Fix.
type cliError struct {
	Exit    int
	Code    string
	Message string
	Fix     string
}

func (e *cliError) Error() string {
	if e.Fix == "" {
		return e.Message
	}
	return e.Message + " — " + e.Fix
}

// usageError marks an error as the caller's mistake rather than the work's.
func usageError(err error) error {
	if err == nil {
		return nil
	}
	var ce *cliError
	if errors.As(err, &ce) {
		return err
	}
	return &cliError{Exit: exitUsage, Code: "usage", Message: err.Error()}
}

// errConfirmationRequired is returned when a command would prompt, but nobody is
// there to answer. Declining silently and exiting 0 used to let a script, a CI job
// or an agent believe the change had been made.
func errConfirmationRequired(fix string) error {
	return &cliError{
		Exit:    exitConfirmationRequired,
		Code:    "confirmation_required",
		Message: "nothing was applied: this needs confirming, and stdin is not a terminal",
		Fix:     fix,
	}
}

// errCoreLayer refuses to add or remove a layer in the core tier. Both are a
// wrong call rather than a failed one: the layer is already here, and stays.
func errCoreLayer(name, verb string) error {
	msg := fmt.Sprintf("%s is a core layer: it is in every repository and cannot be removed", name)
	if verb == "add" {
		msg = fmt.Sprintf("%s is a core layer and is already here: every repository has it", name)
	}
	return &cliError{
		Exit:    exitUsage,
		Code:    "core_layer",
		Message: msg,
		Fix:     fmt.Sprintf("to change what it does, set its variables — `ilk info %s` lists them, `ilk config set %s.<variable> <value>` sets one", name, name),
	}
}

// classify turns any error into a cliError with an exit code.
func classify(err error) *cliError {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce
	}
	var clash *engine.ClashError
	if errors.As(err, &clash) {
		return &cliError{Exit: exitFailed, Code: "layer_conflict", Message: clash.Error(), Fix: clash.Fix()}
	}
	msg := err.Error()
	// Cobra reports unknown commands itself, before any of ilk's code runs.
	if strings.HasPrefix(msg, "unknown command ") {
		return &cliError{Exit: exitUsage, Code: "usage", Message: msg}
	}
	return &cliError{Exit: exitFailed, Code: "failed", Message: msg}
}

// report prints an error and returns the exit code it carries. Under --json the
// error is a single JSON object on stderr, so stdout stays whatever the command
// had already emitted and a caller never has to parse prose to learn what broke.
func report(err error, asJSON bool) int {
	ce := classify(err)
	if asJSON {
		payload := map[string]any{
			"error": map[string]any{
				"code":    ce.Code,
				"message": ce.Message,
				"exit":    ce.Exit,
			},
		}
		if ce.Fix != "" {
			payload["error"].(map[string]any)["fix"] = ce.Fix
		}
		enc := json.NewEncoder(errOut)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(payload)
		return ce.Exit
	}
	fail(ce)
	return ce.Exit
}

// markUsageErrors makes every argument and flag error in the tree exit 2. Cobra
// returns these as plain errors, indistinguishable from a failed run.
func markUsageErrors(c *cobra.Command) {
	c.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError(err) })
	if c.HasParent() && c.HasSubCommands() && !c.Runnable() {
		// Cobra answers an unknown subcommand of a group with the group's help
		// and exit 0, so `ilk agents remove x` would look like it worked. A group
		// prints its help when called bare, and refuses anything else.
		c.RunE = func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return unknownSubcommand(cmd, args[0])
		}
	}
	if c.Args != nil {
		validate := c.Args
		c.Args = func(cmd *cobra.Command, args []string) error {
			return usageError(validate(cmd, args))
		}
	}
	for _, sub := range c.Commands() {
		markUsageErrors(sub)
	}
}

// unknownSubcommand is the usage error for `ilk <group> <name>` when the group
// has no such command, in the words cobra uses for an unknown top-level one.
func unknownSubcommand(group *cobra.Command, name string) error {
	var names []string
	for _, sub := range group.Commands() {
		if sub.IsAvailableCommand() {
			names = append(names, sub.Name())
		}
	}
	msg := fmt.Sprintf("unknown command %q for %q", name, group.CommandPath())
	if group.SuggestionsMinimumDistance <= 0 {
		group.SuggestionsMinimumDistance = 2
	}
	if hints := group.SuggestionsFor(name); len(hints) > 0 {
		msg += " (did you mean " + strings.Join(hints, " or ") + "?)"
	}
	return &cliError{
		Exit:    exitUsage,
		Code:    "usage",
		Message: msg,
		Fix:     fmt.Sprintf("its commands are %s; see `%s --help`", strings.Join(names, ", "), group.CommandPath()),
	}
}
