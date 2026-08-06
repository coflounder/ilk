package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/version"
	"github.com/spf13/cobra"
)

// attachLayerCommands extends the CLI with `ilk <layer> <command>` for every
// adopted layer that ships commands.
//
// This is how a layer extends the default API without ilk knowing anything about
// it. Built-in verbs are registered first and are never shadowed: a layer called
// `check` cannot take over `ilk check`.
func attachLayerCommands(root *cobra.Command) {
	p, err := engine.Load(workdir(), version.String())
	if err != nil {
		// No project, or a broken one. The built-in commands still have to work —
		// `ilk init` and `ilk info` in particular.
		return
	}

	reserved := map[string]bool{}
	for _, c := range root.Commands() {
		reserved[c.Name()] = true
		for _, alias := range c.Aliases {
			reserved[alias] = true
		}
	}

	for _, l := range p.Layers {
		m := l.Loaded.Manifest
		if len(m.Commands) == 0 {
			continue
		}
		if reserved[m.Name()] {
			continue
		}

		group := &cobra.Command{
			Use:   m.Name(),
			Short: m.Summary,
			Long:  fmt.Sprintf("Commands provided by the %s layer.\n\n%s", m.ID, m.Summary),
		}
		for _, c := range m.Commands {
			group.AddCommand(newLayerRunCmd(l, c.Name, c.Summary, c.Run))
		}
		root.AddCommand(group)
	}
}

func newLayerRunCmd(l *engine.ResolvedLayer, name, summary, run string) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Short:              summary,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				return cmd.Help()
			}
			full := run
			if len(args) > 0 {
				full += " " + shellQuoteAll(args)
			}
			c := exec.Command("sh", "-c", full)
			c.Dir = l.Ctx.Repo.Root
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			c.Env = append(os.Environ(),
				"ILK_LAYER="+l.ID(),
				"ILK_REPO_ROOT="+l.Ctx.Repo.Root,
			)
			for k, v := range l.Vars {
				c.Env = append(c.Env, "ILK_VAR_"+strings.ToUpper(k)+"="+v)
			}
			if err := c.Run(); err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
					return nil
				}
				return err
			}
			return nil
		},
	}
}

// shellQuoteAll makes user arguments safe to interpolate into `sh -c`.
func shellQuoteAll(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		quoted = append(quoted, shellQuote(a))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
