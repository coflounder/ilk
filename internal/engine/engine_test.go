package engine

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/basestore"
	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/repo"
)

// The contract this file exists to defend: adding a layer, editing around it,
// upgrading it and removing it must leave a repository exactly as it was, apart
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
//
// A symlink is recorded by where it points rather than by what it points at.
// Following it would both read a directory as a file and, worse, let a link ilk
// forgot to clean up pass as an ordinary absence.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			out[rel] = "-> " + target
			return nil
		}
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
		cfg.Set(config.LayerRef{ID: id})
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

func TestAddThenRemoveRestoresTheRepository(t *testing.T) {
	root := fixture(t)
	before := snapshot(t, root)

	configWith(t, root, "ilk/record", "ilk/gates")
	apply(t, root)

	if !exists(root, "docs/reference/README.md") {
		t.Fatal("add did not create the record directories")
	}
	if !strings.Contains(read(t, root, "AGENTS.md"), "ilk:begin") {
		t.Fatal("add did not write an instruction block")
	}

	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Remove("ilk/record")
	cfg.Remove("ilk/gates")
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
				t.Errorf("%s survived the removal and is not a seeded file", path)
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
	case "docs/reference/README.md", "docs/plans/README.md", "docs/log/README.md",
		"scratch/README.md", ".github/workflows/ilk.yml":
		return true
	}
	return false
}

func TestHumanProseAroundABlockSurvivesUpgradeAndRemoval(t *testing.T) {
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
	cfg.Remove("ilk/record")
	cfg.RemoveTarget("claude-code")
	_ = cfg.Save(root)
	apply(t, root)

	got := read(t, root, "AGENTS.md")
	want := humanAgents + addition
	if got != want {
		t.Fatalf("rm did not restore AGENTS.md:\n got %q\nwant %q", got, want)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	root := fixture(t)
	configWith(t, root, "ilk/record", "ilk/gates")
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

	target := ".agents/skills/write-decision/SKILL.md"
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

func TestRemovingOneLayerLeavesTheOtherIntact(t *testing.T) {
	root := fixture(t)
	configWith(t, root, "ilk/record", "ilk/gates")
	apply(t, root)

	cfg, _ := config.Load(root)
	cfg.Remove("ilk/gates")
	_ = cfg.Save(root)
	apply(t, root)

	agents := read(t, root, "AGENTS.md")
	if !strings.Contains(agents, "layer=ilk/record") {
		t.Error("removing quality-gates took away the record layer's block")
	}
	if strings.Contains(agents, "layer=ilk/gates") {
		t.Error("the removed layer's block is still present")
	}
	if !exists(root, ".agents/skills/write-decision/SKILL.md") {
		t.Error("removing one layer took away another layer's skill")
	}
	if exists(root, ".agents/skills/prove-it/SKILL.md") {
		t.Error("the removed layer's skill is still present")
	}
}

func TestSeededFilesAreNeverOverwritten(t *testing.T) {
	root := fixture(t)
	configWith(t, root, "ilk/record")
	apply(t, root)

	mine := "# My own reference README\n"
	if err := os.WriteFile(filepath.Join(root, "docs/reference/README.md"), []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	apply(t, root)

	if got := read(t, root, "docs/reference/README.md"); got != mine {
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
	cfg.Set(config.LayerRef{ID: "ilk/gates"})
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}

	p, err := Load(root, "test")
	if err != nil {
		t.Fatalf("a layer with an unmet requirement should still load: %v", err)
	}
	missing := p.MissingRequirements()
	if got := missing["ilk/gates"]; len(got) != 1 || got[0] != "test.command" {
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
	if len(lk.Owners) == 0 {
		t.Fatal("the lockfile recorded nothing")
	}
	for _, entry := range lk.Owners {
		for _, f := range entry.Files {
			if f.Path == "" {
				t.Errorf("%s recorded a file with no path", entry.ID)
			}
			if f.Mode == "" {
				t.Errorf("%s recorded %s with no mode", entry.ID, f.Path)
			}
		}
	}
	if _, _, ok := lk.Find("ilk/record", "AGENTS.md", "instructions"); !ok {
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

// A layer governs what happens next, not what came before. These tests defend the
// first-run experience of a repository that already has documentation.

func TestAddingALayerExemptsWhatWasAlreadyThere(t *testing.T) {
	root := fixture(t)
	write(t, root, "docs/reference/getting-started.md", "# Getting started\n")
	write(t, root, "docs/reference/api_reference.md", "# API\n")
	write(t, root, "docs/reference/guides/deploy.md", "# Deploy\n")

	configWith(t, root, "ilk/record")
	apply(t, root)

	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	baseline := p.Baseline()
	for _, path := range []string{"docs/reference/getting-started.md", "docs/reference/api_reference.md", "docs/reference/guides/deploy.md"} {
		if !baseline[path] {
			t.Errorf("%s predates the layer and should be exempt; baseline is %v", path, baseline)
		}
	}
	// Files ilk itself seeded are ilk's problem, not history's.
	if baseline["docs/reference/README.md"] {
		t.Error("a file ilk seeded should not be in the baseline")
	}
}

func TestBaselineIsDecidedOnceAndDoesNotGrow(t *testing.T) {
	root := fixture(t)
	write(t, root, "docs/reference/old.md", "# Old\n")

	configWith(t, root, "ilk/record")
	apply(t, root)

	// A file added afterwards is governed, so it must not be swept into the
	// exemption by a later apply.
	write(t, root, "docs/reference/added_later.md", "# Later\n")
	apply(t, root)

	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	baseline := p.Baseline()
	if !baseline["docs/reference/old.md"] {
		t.Error("the original exemption was lost")
	}
	if baseline["docs/reference/added_later.md"] {
		t.Error("a file created after adoption was quietly exempted")
	}
}

func TestNoBaselineGovernsExistingFilesImmediately(t *testing.T) {
	root := fixture(t)
	write(t, root, "docs/reference/old.md", "# Old\n")

	cfg := config.Default()
	cfg.Targets = []string{"claude-code"}
	cfg.Set(config.LayerRef{ID: "ilk/record"})
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}

	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	pl, err := p.Plan(PlanOptions{Prune: true, NoBaseline: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(pl.Baselines) != 0 {
		t.Fatalf("--no-baseline should exempt nothing, got %v", pl.Baselines)
	}
}

func TestClearingTheBaselineIsSelective(t *testing.T) {
	root := fixture(t)
	write(t, root, "docs/reference/one.md", "# One\n")
	write(t, root, "docs/reference/two.md", "# Two\n")

	configWith(t, root, "ilk/record")
	apply(t, root)

	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	cleared := p.ClearBaseline([]string{"docs/reference/one.md"})
	if len(cleared) != 1 || cleared[0] != "docs/reference/one.md" {
		t.Fatalf("expected to clear docs/one.md, got %v", cleared)
	}
	baseline := p.Baseline()
	if baseline["docs/reference/one.md"] {
		t.Error("docs/reference/one.md is still exempt")
	}
	if !baseline["docs/reference/two.md"] {
		t.Error("clearing one path cleared another")
	}

	if again := p.ClearBaseline([]string{"docs/reference/nonexistent.md"}); len(again) != 0 {
		t.Errorf("clearing a path that was never exempt should report nothing, got %v", again)
	}
}

func TestRemovingALayerForgetsItsBaseline(t *testing.T) {
	root := fixture(t)
	write(t, root, "docs/reference/old.md", "# Old\n")

	configWith(t, root, "ilk/record")
	apply(t, root)

	cfg, _ := config.Load(root)
	cfg.Remove("ilk/record")
	_ = cfg.Save(root)
	apply(t, root)

	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Baseline()) != 0 {
		t.Errorf("removing the layer should forget its exemptions, got %v", p.Baseline())
	}
}

// Upgrading over a file somebody has edited is the case the merge exists for.
// These tests use a layer on disk whose content is changed between applies, which
// is what a real `ilk upgrade` does.

// mutableLayer writes a layer directory whose instruction body the test controls.
func mutableLayer(t *testing.T, dir, body string) string {
	t.Helper()
	manifestText := `id: test/mutable
version: 0.1.0
summary: A layer whose content the test changes between applies.
instructions:
  - id: guidance
    src: instructions/guidance.md
    budget: 40
files:
  - src: files/owned.md
    dest: owned.md
    mode: managed
`
	write(t, dir, "layer.yaml", manifestText)
	write(t, dir, "instructions/guidance.md", body)
	write(t, dir, "files/owned.md", body)
	return dir
}

func addMutable(t *testing.T, root, layerDir string) {
	t.Helper()
	cfg := config.Default()
	cfg.Targets = []string{}
	cfg.Set(config.LayerRef{ID: "test/mutable", Source: layerDir})
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
}

const originalBody = `Line one.
Line two.
Line three.
Line four.
`

func TestUpgradeMergesLayerChangesWithUserEdits(t *testing.T) {
	root := fixture(t)
	layerDir := mutableLayer(t, t.TempDir(), originalBody)
	addMutable(t, root, layerDir)
	apply(t, root)

	if got := read(t, root, "owned.md"); got != originalBody {
		t.Fatalf("unexpected initial content %q", got)
	}

	// The user edits one line of a file ilk owns.
	edited := strings.Replace(originalBody, "Line two.", "Line two, with my note.", 1)
	write(t, root, "owned.md", edited)

	// The layer ships a new version that changes a different line.
	upgraded := strings.Replace(originalBody, "Line four.", "Line four, revised by the layer.", 1)
	mutableLayer(t, layerDir, upgraded)

	pl := apply(t, root)

	var merged bool
	for _, a := range pl.Actions {
		if a.Path == "owned.md" && a.Op == OpMerge {
			merged = true
		}
	}
	if !merged {
		t.Fatalf("expected owned.md to be merged, got %+v", pl.Changes())
	}

	got := read(t, root, "owned.md")
	if !strings.Contains(got, "Line two, with my note.") {
		t.Errorf("the user's edit was lost:\n%s", got)
	}
	if !strings.Contains(got, "Line four, revised by the layer.") {
		t.Errorf("the layer's change was lost:\n%s", got)
	}
}

func TestUpgradeRefusesWhenTheSameLinesCollide(t *testing.T) {
	root := fixture(t)
	layerDir := mutableLayer(t, t.TempDir(), originalBody)
	addMutable(t, root, layerDir)
	apply(t, root)

	// Both sides rewrite the same line.
	write(t, root, "owned.md", strings.Replace(originalBody, "Line two.", "Line two, mine.", 1))
	mutableLayer(t, layerDir, strings.Replace(originalBody, "Line two.", "Line two, theirs.", 1))

	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	pl, err := p.Plan(PlanOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}

	var conflict *Action
	for i := range pl.Actions {
		if pl.Actions[i].Path == "owned.md" && pl.Actions[i].Op == OpConflict {
			conflict = &pl.Actions[i]
		}
	}
	if conflict == nil {
		t.Fatalf("a genuine collision must be refused, got %+v", pl.Changes())
	}
	if !strings.Contains(conflict.Note, "--merge-markers") || !strings.Contains(conflict.Note, "--force") {
		t.Errorf("the refusal should name both ways forward, got %q", conflict.Note)
	}

	if err := p.Apply(pl); err != nil {
		t.Fatal(err)
	}
	if got := read(t, root, "owned.md"); !strings.Contains(got, "Line two, mine.") {
		t.Error("a refused merge must leave the file alone")
	}
}

func TestMergeMarkersWriteBothVersions(t *testing.T) {
	root := fixture(t)
	layerDir := mutableLayer(t, t.TempDir(), originalBody)
	addMutable(t, root, layerDir)
	apply(t, root)

	write(t, root, "owned.md", strings.Replace(originalBody, "Line two.", "Line two, mine.", 1))
	mutableLayer(t, layerDir, strings.Replace(originalBody, "Line two.", "Line two, theirs.", 1))

	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	pl, err := p.Plan(PlanOptions{Prune: true, MergeMarkers: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(pl); err != nil {
		t.Fatal(err)
	}

	got := read(t, root, "owned.md")
	for _, want := range []string{"<<<<<<<", "Line two, mine.", "=======", "Line two, theirs.", ">>>>>>>"} {
		if !strings.Contains(got, want) {
			t.Errorf("marker output missing %q:\n%s", want, got)
		}
	}
}

func TestFencedRegionsMergeToo(t *testing.T) {
	root := fixture(t)
	layerDir := mutableLayer(t, t.TempDir(), originalBody)
	addMutable(t, root, layerDir)

	cfg, _ := config.Load(root)
	cfg.Targets = []string{"claude-code"}
	_ = cfg.Save(root)
	apply(t, root)

	// Edit one line inside the generated AGENTS.md block.
	agents := read(t, root, "AGENTS.md")
	edited := strings.Replace(agents, "Line two.", "Line two, with my note.", 1)
	if edited == agents {
		t.Fatal("fixture assumption broken: the block does not contain the expected line")
	}
	write(t, root, "AGENTS.md", edited)

	// The layer changes a different line of the same block.
	mutableLayer(t, layerDir, strings.Replace(originalBody, "Line four.", "Line four, revised.", 1))
	apply(t, root)

	got := read(t, root, "AGENTS.md")
	if !strings.Contains(got, "Line two, with my note.") {
		t.Errorf("the user's edit inside the block was lost:\n%s", got)
	}
	if !strings.Contains(got, "Line four, revised.") {
		t.Errorf("the layer's change to the block was lost:\n%s", got)
	}
}

func TestUserEditWithNoLayerChangeIsDriftNotAMerge(t *testing.T) {
	root := fixture(t)
	layerDir := mutableLayer(t, t.TempDir(), originalBody)
	addMutable(t, root, layerDir)
	apply(t, root)

	// The layer has not moved; only the user has. Accepting this silently would
	// hand ownership of a managed file over without anybody deciding to.
	write(t, root, "owned.md", originalBody+"A line I added.\n")

	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	pl, err := p.Plan(PlanOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, a := range pl.Conflicts() {
		if a.Path == "owned.md" {
			found = true
			if !strings.Contains(a.Note, "nothing to merge") {
				t.Errorf("the note should explain why this is not a merge, got %q", a.Note)
			}
		}
	}
	if !found {
		t.Fatalf("expected drift to be reported, got %+v", pl.Changes())
	}
}

func TestAncestorsAreStoredAndCollected(t *testing.T) {
	root := fixture(t)
	layerDir := mutableLayer(t, t.TempDir(), originalBody)
	addMutable(t, root, layerDir)
	apply(t, root)

	lk, err := lock.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var checked int
	for _, entry := range lk.Owners {
		for _, f := range entry.Files {
			if f.Hash == "" {
				continue
			}
			if _, ok := basestore.Get(root, f.Hash); !ok {
				t.Errorf("no ancestor stored for %s (%s)", f.Path, f.Hash)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("nothing was tracked, so this test proved nothing")
	}

	// Superseding the content must not leave the old copy behind for ever.
	before := countStored(t, root)
	mutableLayer(t, layerDir, originalBody+"A new line from the layer.\n")
	apply(t, root)
	if after := countStored(t, root); after > before {
		t.Errorf("the ancestor store grew from %d to %d instead of being collected", before, after)
	}
}

func countStored(t *testing.T, root string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(basestore.Dir(root), func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func TestAcceptRecordsYourVersionAsTheNewBaseline(t *testing.T) {
	root := fixture(t)
	layerDir := mutableLayer(t, t.TempDir(), originalBody)
	addMutable(t, root, layerDir)
	apply(t, root)

	mine := originalBody + "A line I want to keep.\n"
	write(t, root, "owned.md", mine)

	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	pl, err := p.Plan(PlanOptions{Prune: true, Accept: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(pl); err != nil {
		t.Fatal(err)
	}

	if got := read(t, root, "owned.md"); got != mine {
		t.Fatalf("--accept must not change the file:\n got %q\nwant %q", got, mine)
	}

	// The next plan sees no drift, because the user's version is now the baseline.
	p2, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	pl2, err := p2.Plan(PlanOptions{Prune: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range pl2.Actions {
		if a.Path == "owned.md" && (a.Op == OpConflict || a.Op.Changes()) {
			t.Fatalf("after --accept the file should be settled, got %s: %s", a.Op, a.Note)
		}
	}

	// And a later layer change still merges on top of the accepted version.
	mutableLayer(t, layerDir, strings.Replace(originalBody, "Line one.", "Line one, revised.", 1))
	apply(t, root)

	got := read(t, root, "owned.md")
	if !strings.Contains(got, "A line I want to keep.") {
		t.Errorf("the accepted edit was lost by a later upgrade:\n%s", got)
	}
	if !strings.Contains(got, "Line one, revised.") {
		t.Errorf("the layer's later change did not land:\n%s", got)
	}
}

// A merged file must survive every subsequent apply. Recording the merged result
// as both "what the file is" and "what the layer delivered" made the next apply
// mistake the merge for an untouched file and overwrite it — silently destroying
// work, which is the exact failure this whole subsystem exists to prevent.
func TestAMergedFileSurvivesRepeatedApplies(t *testing.T) {
	root := fixture(t)
	layerDir := mutableLayer(t, t.TempDir(), originalBody)
	addMutable(t, root, layerDir)
	apply(t, root)

	write(t, root, "owned.md", strings.Replace(originalBody, "Line two.", "Line two, mine.", 1))
	mutableLayer(t, layerDir, strings.Replace(originalBody, "Line four.", "Line four, theirs.", 1))
	apply(t, root)

	merged := read(t, root, "owned.md")
	if !strings.Contains(merged, "Line two, mine.") || !strings.Contains(merged, "Line four, theirs.") {
		t.Fatalf("the merge itself failed:\n%s", merged)
	}

	for i := 0; i < 3; i++ {
		p, err := Load(root, "test")
		if err != nil {
			t.Fatal(err)
		}
		pl, err := p.Plan(PlanOptions{Prune: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range pl.Actions {
			if a.Path == "owned.md" && a.Op.Changes() {
				t.Fatalf("apply %d wanted to %s a settled merge: %s", i+1, a.Op, a.Note)
			}
		}
		if err := p.Apply(pl); err != nil {
			t.Fatal(err)
		}
		if got := read(t, root, "owned.md"); got != merged {
			t.Fatalf("apply %d changed the merged file:\n got %q\nwant %q", i+1, got, merged)
		}
	}
}

// The same trap, one level deeper: a merge, then a further layer change, must
// use the layer's previous version as the ancestor rather than the merged text.
func TestASecondUpgradeMergesAgainstTheLayersPreviousVersion(t *testing.T) {
	root := fixture(t)
	layerDir := mutableLayer(t, t.TempDir(), originalBody)
	addMutable(t, root, layerDir)
	apply(t, root)

	write(t, root, "owned.md", strings.Replace(originalBody, "Line two.", "Line two, mine.", 1))
	v2 := strings.Replace(originalBody, "Line four.", "Line four, v2.", 1)
	mutableLayer(t, layerDir, v2)
	apply(t, root)

	// A third version changes a line neither side has touched.
	v3 := strings.Replace(v2, "Line one.", "Line one, v3.", 1)
	mutableLayer(t, layerDir, v3)
	apply(t, root)

	got := read(t, root, "owned.md")
	for _, want := range []string{"Line one, v3.", "Line two, mine.", "Line four, v2."} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q to survive:\n%s", want, got)
		}
	}
}

// A create-only file is seeded once and then belongs to the repository — including
// the decision to delete it. Resurrecting it would override that decision, and would
// do so on every apply for ever.
func TestASeededFileDeletedByTheUserIsNotRecreated(t *testing.T) {
	root := fixture(t)
	configWith(t, root, "ilk/record")
	apply(t, root)

	seeded := "docs/reference/README.md"
	if !exists(root, seeded) {
		t.Fatalf("setup: %s was not seeded", seeded)
	}
	if err := os.Remove(filepath.Join(root, seeded)); err != nil {
		t.Fatal(err)
	}

	pl := apply(t, root)
	if exists(root, seeded) {
		t.Fatal("a seeded file the user deleted was recreated")
	}
	var noted bool
	for _, a := range pl.Actions {
		if a.Path == seeded && a.Op == OpSkip && strings.Contains(a.Note, "not recreated") {
			noted = true
		}
	}
	if !noted {
		t.Error("the plan should say why the file was left absent")
	}

	// And it stays absent on every subsequent apply.
	apply(t, root)
	if exists(root, seeded) {
		t.Fatal("a later apply resurrected it")
	}
}
