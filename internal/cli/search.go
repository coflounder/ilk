package cli

import (
	"strings"

	"github.com/coflounder/ilk/internal/registry"
	"github.com/spf13/cobra"
)

func newSearchCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "search [terms...]",
		Short: "Find layers you could add",
		Long: `Search the layer index for something to add.

The index is embedded in the binary, so this works offline — and it is a snapshot
taken when this ilk was built. A layer published since then can still be added by
its source (` + "`ilk add gh:owner/repo`" + `); it just will not be listed here.

Nothing in the index is endorsed or audited. Run ` + "`ilk info <ref>`" + ` before
adding anything: it shows what a layer writes, what it costs in always-on
context, and whether it runs code.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := registry.Search(strings.Join(args, " "))
			if err != nil {
				return err
			}

			adopted := map[string]bool{}
			if p, err := project(); err == nil {
				for _, l := range p.Layers {
					adopted[l.ID()] = true
				}
			}

			if !all {
				var kept []registry.Entry
				for _, e := range entries {
					if !adopted[e.ID] {
						kept = append(kept, e)
					}
				}
				entries = kept
			}

			if flagJSON {
				type row struct {
					registry.Entry
					Adopted bool `json:"adopted"`
				}
				out := make([]row, 0, len(entries))
				for _, e := range entries {
					out = append(out, row{Entry: e, Adopted: adopted[e.ID]})
				}
				return emitJSON(out)
			}

			if len(entries) == 0 {
				if len(args) == 0 {
					println(sty.dim("Every listed layer is already here. `ilk search --all` shows them anyway."))
				} else {
					printf("%s\n", sty.dim("Nothing in the index matches that."))
					printf("%s\n", sty.dim("The index is a snapshot; a layer published since this ilk was built"))
					printf("%s\n", sty.dim("can still be added directly: ilk add gh:owner/repo"))
				}
				return nil
			}

			for _, e := range entries {
				mark := "  "
				if adopted[e.ID] {
					mark = sty.green("✓ ")
				}
				source := sty.dim("builtin")
				if !e.Builtin() {
					source = sty.dim(e.Source)
				}
				printf("%s%-24s %s\n", mark, e.ID, source)
				printf("    %s\n", e.Summary)
				var facets []string
				if e.Arc != "" {
					facets = append(facets, "arc="+e.Arc)
				}
				if e.Kind != "" {
					facets = append(facets, "kind="+e.Kind)
				}
				if len(e.Requires) > 0 {
					facets = append(facets, "requires "+strings.Join(e.Requires, ", "))
				}
				if len(facets) > 0 {
					printf("    %s\n", sty.dim(strings.Join(facets, "  ")))
				}
				printf("\n")
			}

			printf("%s\n", sty.dim("`ilk info <ref>` shows what a layer writes before you add it."))
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include layers this repository already has")
	return cmd
}
