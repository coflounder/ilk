package cli

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/render"
	"github.com/spf13/cobra"
)

// newMCPCmd is the single entrypoint every projected MCP entry invokes:
// `ilk mcp run <name>`.
//
// Agent config never names the real server command. It asks ilk, which resolves
// the declaration from the adopted layers, renders its arguments against the
// repository's variables and capabilities, and starts it — so editing a layer's
// server definition never rewrites an agent's config, and a credential named by
// requires_env is checked for presence before the server starts instead of
// surfacing as an opaque connection failure inside the agent.
func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run the MCP servers layers declared",
	}
	cmd.AddCommand(newMCPRunCmd(), newMCPListCmd())
	return cmd
}

func newMCPRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <name>",
		Short: "Start a declared MCP server, stdio attached",
		Args:  requireArgs(1, "ilk mcp run <name>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			p, err := project()
			if err != nil {
				return err
			}

			for _, l := range p.Layers {
				for _, s := range l.Loaded.Manifest.MCP {
					if s.Name != name {
						continue
					}

					if missing := missingMCPEnv(s.RequiresEnv); len(missing) > 0 {
						return fmt.Errorf("mcp server %q needs %s in the environment — export it, then restart the server",
							name, strings.Join(missing, ", "))
					}

					where := l.ID() + ":mcp:" + s.Name
					command, err := render.String(where, s.Command, l.Ctx)
					if err != nil {
						return err
					}
					argv := make([]string, 0, len(s.Args))
					for _, a := range s.Args {
						rendered, err := render.String(where, a, l.Ctx)
						if err != nil {
							return err
						}
						argv = append(argv, rendered)
					}
					env := os.Environ()
					for _, k := range sortedKeys(s.Env) {
						v, err := render.String(where, s.Env[k], l.Ctx)
						if err != nil {
							return err
						}
						env = append(env, k+"="+v)
					}

					proc := exec.Command(command, argv...)
					proc.Dir = p.Repo.Root
					proc.Env = env
					proc.Stdin = os.Stdin
					proc.Stdout = os.Stdout
					proc.Stderr = os.Stderr
					err = proc.Run()
					if exitErr, ok := err.(*exec.ExitError); ok {
						exitCode = exitErr.ExitCode()
						return nil
					}
					return err
				}
			}
			return fmt.Errorf("no adopted layer declares an mcp server named %q — `ilk mcp list` shows what exists", name)
		},
	}
}

func newMCPListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show which layers declared which MCP servers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := project()
			if err != nil {
				return err
			}
			type row struct {
				Name        string   `json:"name"`
				Layer       string   `json:"layer"`
				Summary     string   `json:"summary,omitempty"`
				Command     string   `json:"command"`
				RequiresEnv []string `json:"requires_env,omitempty"`
			}
			var rows []row
			for _, l := range p.Layers {
				for _, s := range l.Loaded.Manifest.MCP {
					rows = append(rows, row{s.Name, l.ID(), s.Summary, s.Command, s.RequiresEnv})
				}
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
			if flagJSON {
				return emitJSON(rows)
			}
			if len(rows) == 0 {
				println(sty.dim("No MCP servers declared."))
				return nil
			}
			for _, r := range rows {
				note := r.Summary
				if missing := missingMCPEnv(r.RequiresEnv); len(missing) > 0 {
					note = "needs " + strings.Join(missing, ", ") + " in the environment"
				}
				printf("%-20s %-24s %s\n", r.Name, sty.dim(r.Layer), sty.dim(note))
			}
			return nil
		},
	}
}

// missingMCPEnv mirrors the check runner's credential rule: presence is tested,
// values are never read.
func missingMCPEnv(names []string) []string {
	var missing []string
	for _, name := range names {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
