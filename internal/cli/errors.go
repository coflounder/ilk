package cli

import (
	"encoding/json"
	"errors"
	"strings"

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

// classify turns any error into a cliError with an exit code.
func classify(err error) *cliError {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce
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
	fail(err)
	return ce.Exit
}

// markUsageErrors makes every argument and flag error in the tree exit 2. Cobra
// returns these as plain errors, indistinguishable from a failed run.
func markUsageErrors(c *cobra.Command) {
	c.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError(err) })
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
