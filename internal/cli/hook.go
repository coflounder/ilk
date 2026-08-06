package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/coflounder/ilk/internal/manifest"
	"github.com/spf13/cobra"
)

// newHookCmd implements the single entrypoint every agent adapter and git hook
// writes: `ilk hook run <event>`.
//
// Because the generated configuration only ever says "ask ilk what to run",
// adding or removing a hook in a layer never rewrites an agent's settings file.
func newHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Run the hooks layers registered for a lifecycle event",
	}
	cmd.AddCommand(newHookRunCmd(), newHookListCmd())
	return cmd
}

func newHookRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <event>",
		Short: "Run every hook registered for an event",
		Long: "Events: " + strings.Join(manifest.Events, ", ") + `

Blocking hooks fail the run when their command exits non-zero, which is what
makes a pre-commit gate a gate. Non-blocking hooks only report.`,
		Args: requireArgs(1, "ilk hook run <event>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			event := args[0]
			if !manifest.ValidEvent(event) {
				return fmt.Errorf("unknown event %q — one of: %s", event, strings.Join(manifest.Events, ", "))
			}

			p, err := project()
			if err != nil {
				// A hook firing in a repository that has no ilk config is not an
				// error worth blocking a commit over.
				return nil
			}

			type job struct {
				layer    string
				name     string
				run      string
				blocking bool
			}
			var jobs []job
			for _, l := range p.Layers {
				for _, h := range l.Loaded.Manifest.Hooks {
					if h.Event != event {
						continue
					}
					name := h.Name
					if name == "" {
						name = l.Name()
					}
					jobs = append(jobs, job{l.ID(), name, h.Run, h.Blocking})
				}
			}
			if len(jobs) == 0 {
				return nil
			}

			failed := 0
			for _, j := range jobs {
				out, code := runShell(p.Repo.Root, j.run)
				if code == 0 {
					if event == "session-start" {
						// The session-start packet is the output, not a status line.
						fmt.Fprint(os.Stdout, out)
					}
					continue
				}
				fmt.Fprintf(os.Stderr, "%s %s (%s)\n", sty.red("hook failed:"), j.name, j.layer)
				fmt.Fprint(os.Stderr, out)
				if j.blocking {
					failed++
				}
			}
			if failed > 0 {
				exitCode = 1
				fmt.Fprintf(os.Stderr, "\n%s %d blocking %s hook(s) failed.\n", sty.red("blocked:"), failed, event)
				fmt.Fprintf(os.Stderr, "%s\n", sty.dim("Fix the failures above, or bypass deliberately with --no-verify."))
			}
			return nil
		},
	}
}

func newHookListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show which hooks are registered for which events",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := project()
			if err != nil {
				return err
			}
			type row struct {
				Event    string `json:"event"`
				Layer    string `json:"layer"`
				Name     string `json:"name"`
				Run      string `json:"run"`
				Blocking bool   `json:"blocking"`
			}
			var rows []row
			for _, event := range manifest.Events {
				for _, l := range p.Layers {
					for _, h := range l.Loaded.Manifest.Hooks {
						if h.Event == event {
							rows = append(rows, row{event, l.ID(), h.Name, h.Run, h.Blocking})
						}
					}
				}
			}
			if flagJSON {
				return emitJSON(rows)
			}
			if len(rows) == 0 {
				println(sty.dim("No hooks registered."))
				return nil
			}
			for _, r := range rows {
				marker := " "
				if r.Blocking {
					marker = sty.red("!")
				}
				printf("%s %-14s %-20s %s\n", marker, r.Event, sty.dim(r.Layer), r.Run)
			}
			printf("\n%s\n", sty.dim("! marks a blocking hook — a non-zero exit stops the action"))
			return nil
		},
	}
}

func runShell(dir, command string) (string, int) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	out, err := cmd.CombinedOutput()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return string(out), exitErr.ExitCode()
	}
	if err != nil {
		return string(out) + err.Error() + "\n", -1
	}
	return string(out), 0
}
