package cli

import (
	"fmt"

	"github.com/coflounder/ilk/internal/brief"
	"github.com/spf13/cobra"
)

func newBriefCmd() *cobra.Command {
	var full bool
	cmd := &cobra.Command{
		Use:   "brief",
		Short: "Print the packet a session should start with",
		Long: `Assemble what an agent needs to know before doing anything here: the
directory contract, the layers in force, the skills available, the commands that
verify claims, and whether the repository currently validates.

This is what the session-start hook runs. It is computed from the repository, so
orientation does not depend on whoever wrote the prompt that morning.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := project()
			if err != nil {
				return err
			}
			b, err := brief.Build(p, brief.Options{Full: full})
			if err != nil {
				return err
			}
			if flagJSON {
				return emitJSON(b)
			}
			renderBrief(b)
			return nil
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "also run command-based checks (slower)")
	return cmd
}

// renderBrief prints markdown, because the consumer is usually a language model
// and markdown is what it reads best.
func renderBrief(b *brief.Brief) {
	printf("# %s\n\n", b.Project)

	if len(b.Contract) > 0 {
		printf("## Where things live\n\n")
		for _, d := range b.Contract {
			printf("- `%s/` — %s", d.Path, d.Purpose)
			if d.Files > 0 {
				printf(" _(%d documents)_", d.Files)
			}
			printf("\n")
		}
		printf("\n")
	}

	if len(b.Recent) > 0 {
		printf("## Recently touched\n\n")
		for _, f := range b.Recent {
			line := "- `" + f.Path + "`"
			if f.Title != "" {
				line += " — " + f.Title
			}
			if f.Status != "" {
				line += fmt.Sprintf(" _(%s", f.Status)
				if f.Updated != "" {
					line += ", updated " + f.Updated
				}
				line += ")_"
			}
			println(line)
		}
		printf("\n")
	}

	if b.Checks != nil {
		printf("## State\n\n")
		switch {
		case b.Checks.Failed == 0 && b.Checks.Errored == 0:
			printf("Validation is clean (%d checks passing).", b.Checks.Passed)
			if b.Checks.Skipped > 0 {
				printf(" %d skipped.", b.Checks.Skipped)
			}
			printf("\n\n")
		default:
			printf("**Validation is failing.** Fix these before building on top of them:\n\n")
			for _, r := range b.Checks.Failing {
				printf("- **%s** — %s\n", r.ID, r.Title)
				for _, f := range r.Findings {
					loc := f.Path
					if f.Line > 0 {
						loc = fmt.Sprintf("%s:%d", f.Path, f.Line)
					}
					if loc != "" {
						printf("  - `%s` %s\n", loc, f.Message)
					} else {
						printf("  - %s\n", f.Message)
					}
				}
				if r.Fix != "" {
					printf("  - _fix:_ %s\n", r.Fix)
				}
			}
			printf("\n")
		}
	}

	if len(b.Skills) > 0 {
		printf("## Procedures available\n\n")
		printf("Read the matching file when its situation applies. Do not read them all up front.\n\n")
		for _, s := range b.Skills {
			printf("- **%s** — %s\n  `%s`\n", s.Name, s.Description, s.Path)
		}
		printf("\n")
	}

	if len(b.Commands) > 0 {
		printf("## Commands\n\n")
		for _, c := range b.Commands {
			printf("- `%s` — %s\n", c.Command, c.Summary)
		}
		printf("\n")
	}

	if len(b.Capabilities) > 0 {
		printf("## Verify your own claims with\n\n")
		for _, c := range b.Capabilities {
			printf("- `%s` (%s)\n", c.Value, c.Name)
		}
		printf("\n")
	}

	if len(b.Layers) > 0 {
		printf("## Layers in force\n\n")
		for _, l := range b.Layers {
			printf("- `%s` %s — %s\n", l.ID, l.Version, l.Summary)
		}
		printf("\n")
	}

	for _, w := range b.Warnings {
		printf("> **Warning:** %s\n", w)
	}
}
