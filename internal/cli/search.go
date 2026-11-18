package cli

import (
	"fmt"
	"strings"

	"github.com/coflounder/ilk/internal/manifest"
	"github.com/coflounder/ilk/internal/registry"
	"github.com/spf13/cobra"
)

func newSearchCmd() *cobra.Command {
	var all bool
	var forTrait string
	cmd := &cobra.Command{
		Use:   "search [terms...]",
		Short: "Find layers you could add",
		Example: `  ilk search
  ilk search plans
  ilk search --for ui
  ilk search --all --json`,
		Long: `Search the layer index for something to add.

The index is embedded in the binary, so this works offline — and it is a snapshot
taken when this ilk was built. A layer published since then can still be added by
its source (` + "`ilk add gh:owner/repo`" + `); it just will not be listed here.

--for narrows the list to the layers meant for one kind of repository: ` + traitList() + `.
` + "`--for any`" + ` lists the layers meant for every repository.

Nothing in the index is endorsed or audited. Run ` + "`ilk info <ref>`" + ` before
adding anything: it shows what a layer writes, what it costs in always-on
context, and whether it runs code.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if forTrait != "" && !manifest.ValidTrait(forTrait) {
				return &cliError{
					Exit:    exitUsage,
					Code:    "usage",
					Message: fmt.Sprintf("%q is not a repository trait", forTrait),
					Fix:     "use one of " + strings.Join(manifest.TraitNames(), ", "),
				}
			}
			entries, err := registry.Search(strings.Join(args, " "))
			if err != nil {
				return err
			}
			if forTrait != "" {
				var kept []registry.Entry
				for _, e := range entries {
					if e.IsFor(forTrait) {
						kept = append(kept, e)
					}
				}
				entries = kept
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
				if printBundles(strings.Join(args, " "), forTrait, adopted, all) {
					return nil
				}
				if forTrait != "" {
					printf("%s\n", sty.dim(fmt.Sprintf("No layer here is declared for %s. `ilk search` without --for shows them all.", forTrait)))
				} else if len(args) == 0 {
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
				if len(e.For) > 0 {
					facets = append(facets, "for "+strings.Join(e.For, ", "))
				}
				if len(e.Requires) > 0 {
					facets = append(facets, "requires "+strings.Join(e.Requires, ", "))
				}
				if len(facets) > 0 {
					printf("    %s\n", sty.dim(strings.Join(facets, "  ")))
				}
				printf("\n")
			}

			printBundles(strings.Join(args, " "), forTrait, adopted, all)
			printf("%s\n", sty.dim("`ilk info <ref>` shows what a layer writes before you add it."))
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include layers this repository already has")
	cmd.Flags().StringVar(&forTrait, "for", "", "only layers meant for this kind of repository: "+strings.Join(manifest.TraitNames(), ", "))
	return cmd
}

// traitList is the repository traits with their meanings, for help text.
func traitList() string {
	var parts []string
	for _, t := range manifest.Traits {
		parts = append(parts, t.Name+" ("+t.Meaning+")")
	}
	return strings.Join(parts, ", ")
}

// bundleIsFor reports whether every member of a bundle is declared for a
// trait, which is when the bundle is too.
func bundleIsFor(b registry.Bundle, trait string) bool {
	for _, id := range b.Layers {
		e, ok := registry.Lookup(id)
		if !ok || !e.IsFor(trait) {
			return false
		}
	}
	return true
}

// printBundles lists the bundles that match the query, leaving out those whose
// every member is already here unless --all asks for them.
func printBundles(query, forTrait string, adopted map[string]bool, all bool) bool {
	bundles, err := registry.Bundles()
	if err != nil || len(bundles) == 0 {
		return false
	}
	terms := strings.Fields(strings.ToLower(query))
	var shown []registry.Bundle
	for _, b := range bundles {
		haystack := strings.ToLower(b.Ref() + " " + b.Summary + " " + strings.Join(b.Layers, " "))
		matched := true
		for _, t := range terms {
			if !strings.Contains(haystack, t) {
				matched = false
				break
			}
		}
		complete := true
		for _, id := range b.Layers {
			if !adopted[id] {
				complete = false
			}
		}
		if forTrait != "" && !bundleIsFor(b, forTrait) {
			matched = false
		}
		if matched && (all || !complete) {
			shown = append(shown, b)
		}
	}
	if len(shown) == 0 {
		return false
	}
	printf("%s\n", sty.bold("Bundles"))
	for _, b := range shown {
		printf("  %-24s %s\n", b.Ref(), sty.dim(strings.Join(b.Layers, ", ")))
		printf("    %s\n\n", b.Summary)
	}
	return true
}
