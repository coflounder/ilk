package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/coflounder/ilk/internal/render"
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

	grouped := false
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
		// The heading exists only when a layer here ships commands; cobra prints
		// a group's title even when nothing is under it.
		if !grouped {
			root.AddGroup(&cobra.Group{ID: groupLayerCmds, Title: "LAYER COMMANDS"})
			grouped = true
		}
		group.GroupID = groupLayerCmds
		for _, c := range m.Commands {
			group.AddCommand(newLayerRunCmd(l, c))
		}
		root.AddCommand(group)
	}
}

func newLayerRunCmd(l *engine.ResolvedLayer, c manifest.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:     layerUseLine(c),
		Short:   c.Summary,
		Long:    fmt.Sprintf("%s\n\nProvided by the %s layer.", c.Summary, l.ID()),
		Example: indentExamples(c.Examples),
		Args:    layerArgsValidator(c),
	}

	// Every declared flag reaches the script, given or not, so a script reads its
	// own defaults from one place: the manifest.
	values := map[string]func() string{}
	for _, f := range c.Flags {
		switch f.Kind() {
		case "bool":
			v := cmd.Flags().BoolP(f.Name, f.Short, f.Default == "true", f.Usage)
			values[f.EnvName()] = func() string {
				if *v {
					return "1"
				}
				return "0"
			}
		default:
			v := &layerFlagValue{kind: f.Kind(), value: f.Default}
			cmd.Flags().VarP(v, f.Name, f.Short, f.Usage)
			values[f.EnvName()] = v.String
		}
	}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if flagJSON && !c.JSON {
			return &cliError{
				Exit:    exitUsage,
				Code:    "json_unsupported",
				Message: fmt.Sprintf("`%s` has no JSON output", cmd.CommandPath()),
				Fix:     fmt.Sprintf("run it without --json; the %s layer offers JSON by declaring `json: true` on the command", l.ID()),
			}
		}
		// Rendered like every other command a layer declares — a check's `run:`,
		// a mirror's provider commands, a contribution's submit. Leaving this
		// one literal meant a subcommand could not reach its own layer's
		// variables, and the failure showed up as `{{ .Vars.x }}` in output.
		full, err := render.String("command:"+l.ID()+":"+c.Name, c.Run, l.Ctx)
		if err != nil {
			return err
		}
		if len(args) > 0 {
			full += " " + shellQuoteAll(args)
		}
		sh := exec.Command("sh", "-c", full)
		sh.Dir = l.Ctx.Repo.Root
		sh.Stdin = os.Stdin
		sh.Stdout = out
		sh.Stderr = errOut
		sh.Env = append(withoutFlagEnv(os.Environ()),
			"ILK_LAYER="+l.ID(),
			"ILK_REPO_ROOT="+l.Ctx.Repo.Root,
			"ILK_OUTPUT="+outputMode(),
		)
		for k, v := range l.Vars {
			sh.Env = append(sh.Env, "ILK_VAR_"+strings.ToUpper(k)+"="+v)
		}
		for name, value := range values {
			sh.Env = append(sh.Env, name+"="+value())
		}
		if err := sh.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
				return nil
			}
			return err
		}
		return nil
	}
	return cmd
}

// layerUseLine is the usage line built from a command's declared arguments.
func layerUseLine(c manifest.Command) string {
	return strings.TrimSpace(c.Name + " " + c.ArgsUsage())
}

// layerArgsValidator holds a layer command to the arguments it declares, so a
// missing or extra one is a usage error before the script ever runs.
func layerArgsValidator(c manifest.Command) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		required, max := 0, len(c.Args)
		for _, a := range c.Args {
			if a.Required {
				required++
			}
			if a.Variadic {
				max = -1
			}
		}
		if len(args) < required {
			return fmt.Errorf("missing <%s> — usage: %s", c.Args[len(args)].Name, cmd.UseLine())
		}
		if max >= 0 && len(args) > max {
			return fmt.Errorf("unexpected argument %q — usage: %s", args[max], cmd.UseLine())
		}
		return nil
	}
}

// indentExamples lays a layer's examples out the way built-in commands' are.
func indentExamples(examples []string) string {
	lines := make([]string, 0, len(examples))
	for _, e := range examples {
		lines = append(lines, "  "+strings.TrimSpace(e))
	}
	return strings.Join(lines, "\n")
}

// withoutFlagEnv drops ILK_FLAG_ variables ilk did not set itself. A layer
// command run from inside another one would otherwise see its caller's flags.
func withoutFlagEnv(env []string) []string {
	kept := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, "ILK_FLAG_") {
			kept = append(kept, kv)
		}
	}
	return kept
}

// layerFlagValue holds a string or int flag of a layer command. Ints are kept as
// text, since they only ever reach the script as text, but are checked on the
// way in so `--max lots` is a usage error rather than the script's problem.
type layerFlagValue struct {
	kind  string
	value string
}

func (v *layerFlagValue) String() string { return v.value }
func (v *layerFlagValue) Type() string   { return v.kind }
func (v *layerFlagValue) Set(s string) error {
	if v.kind == "int" {
		if _, err := strconv.Atoi(s); err != nil {
			return fmt.Errorf("%q is not a whole number", s)
		}
	}
	v.value = s
	return nil
}

// outputMode is what a layer command is told about the output its caller wants.
func outputMode() string {
	if flagJSON {
		return "json"
	}
	return "text"
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
