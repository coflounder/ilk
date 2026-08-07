package cli

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/layer"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/coflounder/ilk/internal/repo"
	"github.com/spf13/cobra"
)

// newLayerCmd holds the authoring tools. Publishing a layer should be about as
// much work as publishing the blog post that described the idea, which means the
// scaffold has to be one command and the test has to prove the thing that
// actually matters: that removing the layer restores the repository.
func newLayerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "layer",
		Short: "Author and test layers",
	}
	cmd.AddCommand(newLayerNewCmd(), newLayerValidateCmd(), newLayerTestCmd())
	return cmd
}

func newLayerNewCmd() *cobra.Command {
	var namespace, dir string
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Scaffold a new layer",
		Args:  requireArgs(1, "ilk layer new <name>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			target := filepath.Join(dir, name)
			if _, err := os.Stat(target); err == nil {
				return fmt.Errorf("%s already exists", target)
			}
			id := namespace + "/" + name

			files := map[string]string{
				"layer.yaml":                    scaffoldManifest(id, name),
				"instructions/guidance.md.tmpl": scaffoldInstructions(name),
				"skills/do-the-thing.md":        scaffoldSkill(name),
				"README.md":                     scaffoldReadme(id, name),
			}
			for rel, content := range files {
				path := filepath.Join(target, rel)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					return err
				}
			}

			if flagJSON {
				return emitJSON(map[string]string{"id": id, "path": target})
			}
			printf("%s %s\n", sty.green("created"), target)
			printf("\n%s\n", sty.bold("Next"))
			printf("  %-42s %s\n", "ilk layer validate "+target, sty.dim("check the manifest"))
			printf("  %-42s %s\n", "ilk layer test "+target, sty.dim("prove add and rm are lossless"))
			printf("  %-42s %s\n", "ilk add ./"+target, sty.dim("try it here"))
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "local", "namespace for the layer id")
	cmd.Flags().StringVar(&dir, "dir", "layers", "directory to create the layer in")
	return cmd
}

func newLayerValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <path>",
		Short: "Check a layer manifest without adding it",
		Args:  requireArgs(1, "ilk layer validate <path>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			loaded, err := layer.Resolve(args[0], ".ilk/cache")
			if err != nil {
				return err
			}
			m := loaded.Manifest

			var problems []string
			for _, f := range m.Files {
				if f.Src == "" {
					continue
				}
				if _, err := loaded.ReadSource(f.Src); err != nil {
					problems = append(problems, fmt.Sprintf("files: %s", err))
				}
			}
			for _, s := range m.Skills {
				if s.Src == "" {
					continue
				}
				if _, err := loaded.ReadSource(s.Src); err != nil {
					problems = append(problems, fmt.Sprintf("skills: %s", err))
				}
			}
			for _, i := range m.Instructions {
				if i.Src == "" {
					continue
				}
				if _, err := loaded.ReadSource(i.Src); err != nil {
					problems = append(problems, fmt.Sprintf("instructions: %s", err))
				}
			}
			if m.Budget() == 0 && len(m.Instructions) > 0 {
				problems = append(problems, "instructions declare no budget — set `budget:` so repositories can see the context cost before adding it")
			}

			if flagJSON {
				return emitJSON(map[string]any{"id": m.ID, "valid": len(problems) == 0, "problems": problems})
			}
			if len(problems) > 0 {
				for _, p := range problems {
					printf("%s %s\n", sty.red("✗"), p)
				}
				exitCode = 1
				return nil
			}
			printf("%s %s %s is valid\n", sty.green("✓"), m.ID, m.Version)
			return nil
		},
	}
}

// stubCapability invents a value for a capability the layer under test requires
// but nothing in the sandbox supplies.
//
// The shape has to match how the capability is used, because a layer will
// interpolate it. A command capability wants something runnable, and `true` is
// the shell's own no-op. Anything else names a place, and `true` there would have
// the layer earnestly writing its templates into a directory called `true/`.
func stubCapability(name string) string {
	if strings.HasSuffix(name, ".command") {
		return "true"
	}
	return path.Join(".ilk", "layer-test", name)
}

// newLayerTestCmd adds a layer to a throwaway repository and then removes it,
// asserting that the repository comes back byte-for-byte. That round trip is the
// contract a layer has to keep, and it is the one users cannot verify for
// themselves without risking their own repository.
func newLayerTestCmd() *cobra.Command {
	var keep bool
	cmd := &cobra.Command{
		Use:   "test <path>",
		Short: "Prove that adding and removing this layer is lossless",
		Args:  requireArgs(1, "ilk layer test <path>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			loaded, err := layer.Resolve(src, ".ilk/cache")
			if err != nil {
				return err
			}

			sandbox, err := os.MkdirTemp("", "ilk-layer-test-*")
			if err != nil {
				return err
			}
			if keep {
				printf("%s %s\n", sty.dim("sandbox:"), sandbox)
			} else {
				defer os.RemoveAll(sandbox)
			}

			// A fixture with content ilk must not disturb.
			fixture := map[string]string{
				"README.md":  "# Fixture\n\nProse a human wrote.\n",
				"AGENTS.md":  "# Fixture\n\nInstructions a human wrote.\n",
				".gitignore": "node_modules\n",
			}
			for rel, content := range fixture {
				if err := os.WriteFile(filepath.Join(sandbox, rel), []byte(content), 0o644); err != nil {
					return err
				}
			}
			if err := os.MkdirAll(filepath.Join(sandbox, ".ilk"), 0o755); err != nil {
				return err
			}

			before, err := snapshot(sandbox)
			if err != nil {
				return err
			}

			r := &repo.Repo{Root: sandbox}
			cfg := config.Default()
			cfg.Targets = []string{"claude-code"}
			for _, req := range loaded.Manifest.Requires {
				cfg.Capabilities[req] = stubCapability(req)
			}
			cfg.Set(config.LayerRef{ID: loaded.Manifest.ID, Version: loaded.Manifest.Version, Source: src})
			if err := cfg.Save(sandbox); err != nil {
				return err
			}

			p, err := engine.NewProject(r, cfg, lock.New(), "test")
			if err != nil {
				return err
			}
			addPlan, err := p.Plan(engine.PlanOptions{Prune: true})
			if err != nil {
				return err
			}
			if err := p.Apply(addPlan); err != nil {
				return err
			}

			// Files the layer only seeds are handed over on purpose. Record them
			// now so the round-trip check does not mistake a deliberate hand-over
			// for residue.
			seeded := map[string]bool{}
			for _, a := range addPlan.Actions {
				if a.Mode == manifest.ModeCreateOnly {
					seeded[a.Path] = true
				}
			}

			// Adopt must be idempotent: a second apply changes nothing.
			p2, err := engine.Load(sandbox, "test")
			if err != nil {
				return err
			}
			again, err := p2.Plan(engine.PlanOptions{Prune: true})
			if err != nil {
				return err
			}
			idempotent := again.Empty()

			// Now remove it and compare against the fixture.
			cfg2, err := config.Load(sandbox)
			if err != nil {
				return err
			}
			cfg2.Remove(loaded.Manifest.ID)
			// Agent projections are ilk's output, not the layer's. Removing them
			// too keeps this measuring the thing it claims to measure: whether
			// *this layer* comes out cleanly.
			cfg2.Targets = nil
			if err := cfg2.Save(sandbox); err != nil {
				return err
			}
			p3, err := engine.Load(sandbox, "test")
			if err != nil {
				return err
			}
			rmPlan, err := p3.Plan(engine.PlanOptions{Prune: true})
			if err != nil {
				return err
			}
			if err := p3.Apply(rmPlan); err != nil {
				return err
			}

			after, err := snapshot(sandbox)
			if err != nil {
				return err
			}
			residue, handed := diffSnapshots(before, after, seeded)

			result := map[string]any{
				"id":          loaded.Manifest.ID,
				"added":       len(addPlan.Changes()),
				"idempotent":  idempotent,
				"residue":     residue,
				"handed_over": handed,
				"lossless":    len(residue) == 0,
				"conflicts":   len(addPlan.Conflicts()),
			}
			if flagJSON {
				if len(residue) > 0 || !idempotent {
					exitCode = 1
				}
				return emitJSON(result)
			}

			printf("%s %s\n\n", sty.bold(loaded.Manifest.ID), sty.dim(loaded.Manifest.Version))
			printf("  %s %d artifact(s)\n", pass(true), len(addPlan.Changes()))
			printf("  %s add is idempotent\n", pass(idempotent))
			printf("  %s rm restores the repository\n", pass(len(residue) == 0))
			for _, r := range residue {
				printf("      %s %s\n", sty.red("left behind:"), r)
			}
			for _, h := range handed {
				printf("  %s %s %s\n", sty.dim("·"), sty.dim("handed over:"), sty.dim(h))
			}
			if len(residue) > 0 || !idempotent {
				exitCode = 1
				printf("\n%s\n", sty.dim("A layer that cannot be cleanly removed is one nobody can safely try."))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&keep, "keep", false, "keep the sandbox directory for inspection")
	return cmd
}

func pass(ok bool) string {
	if ok {
		return sty.green("✓")
	}
	return sty.red("✗")
}

// snapshot records the content of every file in a tree, ignoring ilk's own state
// because config and lock legitimately change.
func snapshot(root string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			if rel == ".ilk" || rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		out[rel] = string(data)
		return nil
	})
	return out, err
}

// diffSnapshots separates residue — anything the layer left behind that it should
// not have — from files it deliberately seeded and handed over to the repository.
func diffSnapshots(before, after map[string]string, seeded map[string]bool) (residue, handed []string) {
	var out []string
	for path, content := range after {
		prev, existed := before[path]
		switch {
		case !existed:
			if seeded[path] {
				handed = append(handed, path)
				continue
			}
			out = append(out, path+" (created and not removed)")
		case prev != content:
			// ilk keeps its own .gitignore block for as long as .ilk/ exists.
			// That is ilk's residue, not the layer's.
			if path == ".gitignore" && onlyCoreBlockAdded(prev, content) {
				continue
			}
			out = append(out, path+" (modified and not restored)")
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			out = append(out, path+" (deleted)")
		}
	}
	sort.Strings(out)
	sort.Strings(handed)
	return out, handed
}

func scaffoldManifest(id, name string) string {
	return fmt.Sprintf(`id: %s
version: 0.1.0
summary: One line describing what adding this layer gets you.
facets:
  arc: quality        # context | planning | execution | quality | release | operations
  kind: process       # record | process | gate | harness | integration | target
ilk: ">=0.1.0"

# Capabilities, not layer names. Anything that supplies these satisfies them.
# requires:
#   - test.command
# provides:
#   - %s.done

variables:
  greeting:
    default: hello
    description: Replace this with something your templates actually need.

# Always-on guidance. Keep it short and declare its cost — an unbounded
# instruction file measurably makes agents worse.
instructions:
  - id: guidance
    src: instructions/guidance.md.tmpl
    budget: 80

# Detail belongs here, loaded only when its situation applies.
skills:
  - name: do-the-thing
    description: Replace this with the situation that should make an agent read the skill.
    src: skills/do-the-thing.md

# checks:
#   - id: %s.example
#     title: Something that must be true
#     run: "test -f README.md"
#     fix: "Create a README.md at the repository root."

# hooks:
#   - event: pre-commit
#     blocking: true
#     run: ilk check --only %s.example
`, id, name, name, name)
}

func scaffoldInstructions(name string) string {
	return fmt.Sprintf(`<!-- Rendered into AGENTS.md. Keep it under the budget declared in layer.yaml. -->
This repository uses the %s layer. Replace this paragraph with the one or two
things an agent genuinely cannot infer from reading the repository itself.

Restating what the code already shows is worse than saying nothing.
`, name)
}

func scaffoldSkill(name string) string {
	return fmt.Sprintf(`# Do the thing

Replace this with a procedure worth following.

## When

The situation that should make an agent open this file. Be specific — this is
what the description in layer.yaml is matched against.

## Procedure

1. The first step.
2. The second step.
3. How to verify the result, with the command that proves it.

## What not to do

The mistakes this procedure exists to prevent. This section is usually the most
valuable one, because it encodes what someone learned the hard way in %s.
`, name)
}

func scaffoldReadme(id, name string) string {
	return fmt.Sprintf(`# %s

One paragraph on the problem this layer solves.

## Add

`+"```"+`
ilk add gh:you/%s
`+"```"+`

## What it adds

Describe the directories, instructions, skills, checks and hooks, so somebody can
decide whether to add it without reading the manifest.

## Remove

`+"```"+`
ilk rm %s
`+"```"+`

Everything this layer added is removed. Anything you edited afterwards is left
alone.
`, id, name, name)
}

// onlyCoreBlockAdded reports whether the sole difference is ilk's always-on block.
func onlyCoreBlockAdded(before, after string) bool {
	if !strings.HasPrefix(after, before) {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(after, before))
	return strings.HasPrefix(rest, "# ilk:begin layer="+engine.CoreOwner)
}
