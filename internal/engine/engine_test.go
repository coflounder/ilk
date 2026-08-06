package engine

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/repo"
)

// The contract this file exists to defend: adopting a layer, editing around it,
// upgrading it and dropping it must leave a repository exactly as it was, apart
// from the files ilk deliberately seeded and handed over.

const humanAgents = `# Fixture

Instructions a human wrote, before any of this.
`

const humanIgnore = "node_modules\n.env\n"

// fixture builds a repository with content ilk must not disturb.
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "README.md", "# Fixture\n\nProse.\n")
	write(t, root, "AGENTS.md", humanAgents)
	write(t, root, ".gitignore", humanIgnore)
	write(t, root, "src/main.go", "package main\n")
	if err := os.MkdirAll(filepath.Join(root, ".ilk"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

func exists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, rel))
	return err == nil
}

// snapshot records every file outside ilk's own state.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel == ".ilk" || rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func configWith(t *testing.T, root string, layers ...string) *config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.Targets = []string{"claude-code"}
	cfg.Capabilities["test.command"] = "true"
	for _, id := range layers {
		cfg.Adopt(config.LayerRef{ID: id})
	}
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// apply reconciles the repository and returns the plan that was applied.
func apply(t *testing.T, root string) *Plan {
	t.Helper()
	p, err := Load(root, "test")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	pl, err := p.Plan(PlanOptions{Prune: true})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if err := p.Apply(pl); err != nil {
		t.Fatalf("apply: %v", err)
	}
	return pl
}

func TestAdoptThenDropRestoresTheRepository(t *testing.T) {
	root := fixture(t)
	before := snapshot(t, root)

	configWith(t, root, "ilk/record", "ilk/quality-gates")
	apply(t, root)

	if !exists(root, "docs/README.md") {
		t.Fatal("adopt did not create the record directories")
	}
	if !strings.Contains(read(t, root, "AGENTS.md"), "ilk:begin") {
		t.Fatal("adopt did not write an instruction block")
	}

	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Drop("ilk/record")
	cfg.Drop("ilk/quality-gates")
	cfg.RemoveTarget("claude-code")
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	apply(t, root)

	after := snapshot(t, root)

	// Only files ilk seeded and handed over may remain, and nothing it wrote may
	// still be present in a file it merely borrowed.
	for path, content := range after {
		prev, existed := before[path]
		switch {
		case !existed:
			if !isSeeded(path) {
				t.Errorf("%s survived the drop and is not a seeded file", path)
			}
		case prev != content:
			// .gitignore keeps ilk's own block for as long as .ilk/ exists: the
			// cache directory still needs ignoring. Everything the user wrote must
			// be untouched, and nothing but that block may have been added.
			if path == ".gitignore" && onlyAddedCoreBlock(prev, content) {
				continue
			}
			t.Errorf("%s was modified and not restored:\n got %q\nwant %q", path, content, prev)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			t.Errorf("%s was deleted; ilk never created it", path)
		}
	}
}

// isSeeded reports whether a leftover file is one ilk deliberately hands over.
func isSeeded(path string) bool {
	switch path {
	case "docs/README.md", "plans/README.md", "log/README.md", "scratch/README.md",
		".github/workflows/ilk.yml":
		return true
	}
	return false
}

func TestHumanProseAroundABlockSurvivesUpgradeAndDrop(t *testing.T) {
	root := fixture(t)
	configWith(t, root, "ilk/record")
	apply(t, root)

	// The human adds prose after ilk's block, which is the normal case: AGENTS.md
	// is theirs, and ilk is a guest in it.
	addition := "\n## House rules\n\nWe do not use mocks.\n"
	agents := read(t, root, "AGENTS.md") + addition
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}

	// An upgrade rewrites the block and must not touch the prose.
	apply(t, root)
	if got := read(t, root, "AGENTS.md"); !strings.Contains(got, "We do not use mocks.") {
		t.Fatalf("upgrade lost the human's prose:\n%s", got)
	}

	cfg, _ := config.Load(root)
	cfg.Drop("ilk/record")
	cfg.RemoveTarget("claude-code")
	_ = cfg.Save(root)
	apply(t, root)

	got := read(t, root, "AGENTS.md")
	want := humanAgents + addition
	if got != want {
		t.Fatalf("drop did not restore AGENTS.md:\n got %q\nwant %q", got, want)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	root := fixture(t)
	configWith(t, root, "ilk/record", "ilk/quality-gates")
	apply(t, root)
	first := snapshot(t, root)

	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	pl, err := p.Plan(PlanOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if !pl.Empty() {
		var ops []string
		for _, a := range pl.Changes() {
			ops = append(ops, string(a.Op)+" "+a.Path)
		}
		sort.Strings(ops)
		t.Fatalf("a second plan wanted changes: %s", strings.Join(ops, ", "))
	}

	apply(t, root)
	if second := snapshot(t, root); len(second) != len(first) {
		t.Fatalf("a second apply changed the tree: %d files became %d", len(first), len(second))
	}
}

func TestEditingAManagedFileIsRefusedRatherThanOverwritten(t *testing.T) {
	root := fixture(t)
	configWith(t, root, "ilk/record")
	apply(t, root)

	target := ".agent/skills/write-decision/SKILL.md"
	edited := read(t, root, target) + "\n<!-- a human's note -->\n"
	if err := os.WriteFile(filepath.Join(root, target), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	pl, err := p.Plan(PlanOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	conflicts := pl.Conflicts()
	if len(conflicts) != 1 || conflicts[0].Path != target {
		t.Fatalf("expected exactly one conflict on %s, got %+v", target, conflicts)
	}
	if !strings.Contains(conflicts[0].Note, "--force") {
		t.Errorf("the conflict should say how to proceed, got %q", conflicts[0].Note)
	}

	if err := p.Apply(pl); err != nil {
		t.Fatal(err)
	}
	if got := read(t, root, target); got != edited {
		t.Error("apply overwrote a file the human had edited")
	}

	// --force is the deliberate escape hatch.
	p2, _ := Load(root, "test")
	forced, err := p2.Plan(PlanOptions{Prune: true, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := p2.Apply(forced); err != nil {
		t.Fatal(err)
	}
	if got := read(t, root, target); got == edited {
		t.Error("--force did not overwrite the edited file")
	}
}

func TestEditingInsideABlockIsRefused(t *testing.T) {
	root := fixture(t)
	configWith(t, root, "ilk/record")
	apply(t, root)

	agents := read(t, root, "AGENTS.md")
	tampered := strings.Replace(agents, "This repository keeps", "TAMPERED keeps", 1)
	if tampered == agents {
		t.Fatal("test fixture assumption broken")
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	p, _ := Load(root, "test")
	pl, err := p.Plan(PlanOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range pl.Conflicts() {
		if c.Path == "AGENTS.md" && c.Region == "instructions" {
			found = true
		}
	}
	if !found {
		t.Fatal("an edit inside a generated block should be reported, not silently overwritten")
	}
}

func TestDroppingOneLayerLeavesTheOtherIntact(t *testing.T) {
	root := fixture(t)
	configWith(t, root, "ilk/record", "ilk/quality-gates")
	apply(t, root)

	cfg, _ := config.Load(root)
	cfg.Drop("ilk/quality-gates")
	_ = cfg.Save(root)
	apply(t, root)

	agents := read(t, root, "AGENTS.md")
	if !strings.Contains(agents, "layer=ilk/record") {
		t.Error("dropping quality-gates removed the record layer's block")
	}
	if strings.Contains(agents, "layer=ilk/quality-gates") {
		t.Error("the dropped layer's block is still present")
	}
	if !exists(root, ".agent/skills/write-decision/SKILL.md") {
		t.Error("dropping one layer removed another layer's skill")
	}
	if exists(root, ".agent/skills/prove-it/SKILL.md") {
		t.Error("the dropped layer's skill is still present")
	}
}

func TestSeededFilesAreNeverOverwritten(t *testing.T) {
	root := fixture(t)
	configWith(t, root, "ilk/record")
	apply(t, root)

	mine := "# My own docs README\n"
	if err := os.WriteFile(filepath.Join(root, "docs/README.md"), []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	apply(t, root)

	if got := read(t, root, "docs/README.md"); got != mine {
		t.Fatalf("a create-only file was overwritten:\n%s", got)
	}
}

func TestGitignoreGainsOneBlockAndKeepsTheUsersLines(t *testing.T) {
	root := fixture(t)
	configWith(t, root, "ilk/record")
	apply(t, root)
	apply(t, root) // append-once must not append twice

	got := read(t, root, ".gitignore")
	if !strings.HasPrefix(got, humanIgnore) {
		t.Errorf("the user's ignore rules were disturbed:\n%s", got)
	}
	if n := strings.Count(got, "scratch/"); n != 1 {
		t.Errorf("scratch/ was ignored %d times, want 1:\n%s", n, got)
	}
}

func TestCapabilitiesComeFromConfigAndLayers(t *testing.T) {
	root := fixture(t)
	configWith(t, root, "ilk/record")
	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	caps := p.Capabilities()
	if caps["test.command"] != "true" {
		t.Errorf("declared capability missing: %v", caps)
	}
	if _, ok := caps["record.docs"]; !ok {
		t.Errorf("a capability provided by an adopted layer should be visible: %v", caps)
	}
}

func TestMissingRequirementIsReportedNotFatal(t *testing.T) {
	root := fixture(t)
	cfg := config.Default()
	cfg.Targets = []string{"claude-code"}
	cfg.Adopt(config.LayerRef{ID: "ilk/quality-gates"})
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}

	p, err := Load(root, "test")
	if err != nil {
		t.Fatalf("a layer with an unmet requirement should still load: %v", err)
	}
	missing := p.MissingRequirements()
	if got := missing["ilk/quality-gates"]; len(got) != 1 || got[0] != "test.command" {
		t.Fatalf("expected test.command to be reported missing, got %v", missing)
	}

	pl, err := p.Plan(PlanOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(pl.Warnings) == 0 {
		t.Error("the plan should warn about the unmet requirement")
	}
	if !strings.Contains(pl.Warnings[0], "capabilities:") {
		t.Errorf("the warning should say how to fix it, got %q", pl.Warnings[0])
	}
}

func TestLockRecordsProvenanceForEveryArtifact(t *testing.T) {
	root := fixture(t)
	configWith(t, root, "ilk/record")
	apply(t, root)

	lk, err := lock.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(lk.Layers) == 0 {
		t.Fatal("the lockfile recorded nothing")
	}
	for _, entry := range lk.Layers {
		for _, f := range entry.Files {
			if f.Path == "" {
				t.Errorf("%s recorded a file with no path", entry.ID)
			}
			if f.Mode == "" {
				t.Errorf("%s recorded %s with no mode", entry.ID, f.Path)
			}
		}
	}
	if _, _, ok := lk.Find("AGENTS.md", "instructions"); !ok {
		t.Error("the AGENTS.md instruction block is not tracked")
	}
}

func TestRepoRootIsFoundFromASubdirectory(t *testing.T) {
	root := fixture(t)
	sub := filepath.Join(root, "src", "deep", "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := repo.Find(sub)
	if err != nil {
		t.Fatal(err)
	}
	// t.TempDir may hand back a symlinked path on macOS; compare resolved forms.
	want, _ := filepath.EvalSymlinks(root)
	got, _ := filepath.EvalSymlinks(r.Root)
	if got != want {
		t.Errorf("found root %q, want %q", got, want)
	}
}

// onlyAddedCoreBlock reports whether the only change to a file is the presence of
// ilk's own always-on block, with the original content preserved verbatim above it.
func onlyAddedCoreBlock(before, after string) bool {
	if !strings.HasPrefix(after, before) {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(after, before))
	return strings.HasPrefix(rest, "# ilk:begin layer="+CoreOwner) &&
		strings.HasSuffix(rest, "# ilk:end layer="+CoreOwner+" region=core")
}
