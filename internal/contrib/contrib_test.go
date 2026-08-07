package contrib

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/basestore"
	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/layer"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/coflounder/ilk/internal/render"
	"github.com/coflounder/ilk/internal/repo"
)

// The flywheel only turns if a proposal is worth reading. These tests hold the
// three things that decide that: the evidence is what ilk actually recorded rather
// than a guess, the patch is against the layer's own source so a maintainer can
// apply it, and nothing leaves the repository that should not.

// fixture builds a repository that has adopted a layer and then tuned it.
func fixture(t *testing.T) (*engine.Project, *engine.ResolvedLayer) {
	t.Helper()
	root := t.TempDir()

	source := filepath.Join(root, "layersrc")
	mustWrite(t, filepath.Join(source, "layer.yaml"), `id: test/demo
version: 0.1.0
summary: A layer that exists to be tuned and then complained about.
variables:
  greeting:
    default: hello
    description: What the script says.
files:
  - src: bin/demo.sh
    dest: .ilk/bin/demo.sh
    mode: managed
skills:
  - name: use-the-demo
    description: How to use the demo.
    src: skills/use-the-demo.md
contribution:
  repo: acme/layers
  path: layers/demo
  guidelines: CONTRIBUTING.md
`)
	mustWrite(t, filepath.Join(source, "bin", "demo.sh"), "#!/bin/sh\nset -eu\necho done\n")
	mustWrite(t, filepath.Join(source, "skills", "use-the-demo.md"), "# Use the demo\n\nRun it.\n")
	mustWrite(t, filepath.Join(source, "CONTRIBUTING.md"), "Say what your toolchain was.\n")

	mustWrite(t, filepath.Join(root, ".ilk", "config.yaml"), "version: 1\ntargets: []\nlayers: []\n")

	cfg := config.Default()
	cfg.Targets = nil
	cfg.Layers = []config.LayerRef{{ID: "test/demo", Source: source, AllowExec: true}}

	p, err := engine.NewProject(&repo.Repo{Root: root}, cfg, lock.New(), "test")
	if err != nil {
		t.Fatal(err)
	}

	pl, err := p.Plan(engine.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(pl); err != nil {
		t.Fatal(err)
	}

	// Reload so the project sees the lockfile the apply just wrote.
	p2, err := engine.NewProject(&repo.Repo{Root: root}, cfg, mustLoadLock(t, root), "test")
	if err != nil {
		t.Fatal(err)
	}
	return p2, p2.Layers[0]
}

func mustLoadLock(t *testing.T, root string) *lock.Lock {
	t.Helper()
	lk, err := lock.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return lk
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func tune(t *testing.T, p *engine.Project, rel, old, new string) {
	t.Helper()
	abs := p.Repo.Path(rel)
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), old, new, 1)
	if updated == string(data) {
		t.Fatalf("%s does not contain %q, so the test is not tuning anything", rel, old)
	}
	if err := os.WriteFile(abs, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
}

func edit(prop *Proposal, path string) (Edit, bool) {
	for _, e := range prop.Edits {
		if e.Path == path {
			return e, true
		}
	}
	return Edit{}, false
}

func TestAnEditedArtifactIsReported(t *testing.T) {
	p, l := fixture(t)
	tune(t, p, ".ilk/bin/demo.sh", "echo done", "echo done twice")

	prop, err := Build(p, l)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := edit(prop, ".ilk/bin/demo.sh")
	if !ok {
		t.Fatalf("the edit was not reported: %+v", prop.Edits)
	}
	if !strings.Contains(e.Diff, "+echo done twice") {
		t.Fatalf("the diff does not carry the change:\n%s", e.Diff)
	}
}

func TestAnUntouchedLayerHasNothingToSay(t *testing.T) {
	p, l := fixture(t)

	prop, err := Build(p, l)
	if err != nil {
		t.Fatal(err)
	}
	// A repository using a layer exactly as shipped has learned that the defaults
	// were right, which is worth knowing and is not a pull request.
	if !prop.Empty() {
		t.Fatalf("expected nothing to report, got %d edit(s) and %d signal(s)", len(prop.Edits), len(prop.Signals))
	}
}

func TestThePatchIsAgainstTheLayersOwnSource(t *testing.T) {
	p, l := fixture(t)
	tune(t, p, ".ilk/bin/demo.sh", "echo done", "echo done twice")

	prop, err := Build(p, l)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := edit(prop, ".ilk/bin/demo.sh")
	if e.Source != "bin/demo.sh" {
		t.Fatalf("source is %q, want bin/demo.sh", e.Source)
	}
	// A maintainer should not have to translate `.ilk/bin/demo.sh` back into
	// `bin/demo.sh` in their head before applying anything.
	if !strings.Contains(e.Diff, "--- a/bin/demo.sh") {
		t.Fatalf("the patch is not against the layer's source path:\n%s", e.Diff)
	}
}

func TestATemplatedArtifactIsEvidenceRatherThanAPatch(t *testing.T) {
	p, l := fixture(t)
	// Give the layer a templated file, so what was delivered is not what the layer
	// holds. A patch built from it would carry this repository's values upstream.
	mustWrite(t, filepath.Join(l.Loaded.Source, "bin", "demo.sh"),
		"#!/bin/sh\nset -eu\necho {{ .Vars.greeting }}\n")
	p, l = reapply(t, p)
	tune(t, p, ".ilk/bin/demo.sh", "echo hello", "echo hello there")

	prop, err := Build(p, l)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := edit(prop, ".ilk/bin/demo.sh")
	if !ok {
		t.Fatal("the edit was not reported")
	}
	if e.Portable {
		t.Fatal("a rendered file was reported as replayable upstream")
	}
	if prop.Portable() {
		t.Fatal("the proposal claims every patch applies cleanly")
	}
	if !strings.Contains(prop.Render(), "rendered, not verbatim") {
		t.Fatal("the document does not warn that the patch carries local values")
	}
}

func TestASkillEditIsAttributedToItsLayer(t *testing.T) {
	p, l := fixture(t)
	// A skill is written by an agent target, not by the layer, so going by
	// ownership alone would make the single most valuable thing an adopter can
	// improve invisible to the layer that shipped it.
	p, l = withTarget(t, p)
	tune(t, p, ".agent/skills/use-the-demo/SKILL.md", "Run it.", "Run it, then read the output.")

	prop, err := Build(p, l)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := edit(prop, ".agent/skills/use-the-demo/SKILL.md")
	if !ok {
		t.Fatalf("the skill edit was not attributed to the layer: %+v", prop.Edits)
	}
	// The target's generated frontmatter is not the layer's content and nobody
	// upstream can change it, so it must not appear in the patch.
	if strings.Contains(e.Diff, "ilk_layer:") {
		t.Fatalf("the patch includes the target's generated header:\n%s", e.Diff)
	}
	if !e.Portable {
		t.Fatalf("a verbatim skill was not reported as replayable:\n%s", e.Diff)
	}
	if !strings.Contains(e.Diff, "--- a/skills/use-the-demo.md") {
		t.Fatalf("the patch is not against the layer's source:\n%s", e.Diff)
	}
}

func TestTheSameSkillIsNotReportedOncePerAgent(t *testing.T) {
	p, l := fixture(t)
	p, l = withTarget(t, p, "agents-md", "claude-code")
	tune(t, p, ".agent/skills/use-the-demo/SKILL.md", "Run it.", "Run it carefully.")
	tune(t, p, ".claude/skills/use-the-demo/SKILL.md", "Run it.", "Run it carefully.")

	prop, err := Build(p, l)
	if err != nil {
		t.Fatal(err)
	}
	// Showing the maintainer the same edit twice suggests a disagreement that is
	// not there.
	if n := len(prop.Edits); n != 1 {
		t.Fatalf("got %d edits for one skill, want 1: %+v", n, prop.Edits)
	}
}

func TestAnOverriddenDefaultIsReported(t *testing.T) {
	p, _ := fixture(t)
	p.Config.Layers[0].Vars = map[string]string{"greeting": "goodbye"}
	p2, err := engine.NewProject(p.Repo, p.Config, p.Lock, "test")
	if err != nil {
		t.Fatal(err)
	}

	prop, err := Build(p2, p2.Layers[0])
	if err != nil {
		t.Fatal(err)
	}
	// One repository changing a default is a preference. The same override in
	// proposal after proposal is a default that is simply wrong, and upstream can
	// only see that pattern if each one is reported.
	found := false
	for _, s := range prop.Signals {
		if s.Kind == SignalVariable && s.Subject == "greeting" {
			found = true
			if !strings.Contains(s.Detail, "hello") || !strings.Contains(s.Detail, "goodbye") {
				t.Fatalf("the signal does not name both values: %s", s.Detail)
			}
		}
	}
	if !found {
		t.Fatalf("the overridden default was not reported: %+v", prop.Signals)
	}
}

func TestASuppliedRequiredVariableIsNotADisagreement(t *testing.T) {
	p, _ := fixture(t)
	mustWrite(t, filepath.Join(p.Layers[0].Loaded.Source, "layer.yaml"), `id: test/demo
version: 0.1.0
summary: A layer with a variable the adopter must supply.
variables:
  owner:
    default: ""
    description: Required; there is no sensible default.
`)
	p.Config.Layers[0].Vars = map[string]string{"owner": "acme"}
	p2, err := engine.NewProject(p.Repo, p.Config, p.Lock, "test")
	if err != nil {
		t.Fatal(err)
	}

	prop, err := Build(p2, p2.Layers[0])
	if err != nil {
		t.Fatal(err)
	}
	// Setting a variable the layer refuses to guess at is using the layer, not
	// disagreeing with it. Reporting it would bury the real signals.
	for _, s := range prop.Signals {
		if s.Kind == SignalVariable {
			t.Fatalf("a required variable was reported as friction: %+v", s)
		}
	}
}

func TestACredentialBlocksSubmission(t *testing.T) {
	p, l := fixture(t)
	tune(t, p, ".ilk/bin/demo.sh", "echo done",
		"echo done # token=ghp_abcdefghijklmnopqrstuvwxyz012345")

	prop, err := Build(p, l)
	if err != nil {
		t.Fatal(err)
	}
	blocking := prop.Blocking()
	if len(blocking) == 0 {
		t.Fatal("a token in a diff was not blocked; a proposal is public and git history is permanent")
	}
	if !strings.Contains(blocking[0].Reason, "rotate") {
		t.Fatalf("the finding does not say to rotate it: %s", blocking[0].Reason)
	}

	if _, err := Draft(p, prop); err != nil {
		t.Fatal(err)
	}
	_, blockers, err := Ready(p, prop)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSubstring(blockers, "rotate") {
		t.Fatalf("submission was not blocked: %v", blockers)
	}
}

func TestNothingIsStrippedFromTheEvidence(t *testing.T) {
	p, l := fixture(t)
	tune(t, p, ".ilk/bin/demo.sh", "echo done", "echo done # see /home/someone/notes")

	prop, err := Build(p, l)
	if err != nil {
		t.Fatal(err)
	}
	if len(prop.Concerns) == 0 {
		t.Fatal("an absolute path was not raised")
	}
	// Editing evidence on the way out would change what upstream is being asked to
	// judge. Raising it is the whole intervention.
	e, _ := edit(prop, ".ilk/bin/demo.sh")
	if !strings.Contains(e.Diff, "/home/someone/notes") {
		t.Fatalf("the evidence was altered:\n%s", e.Diff)
	}
	if len(prop.Blocking()) != 0 {
		t.Fatal("an absolute path blocked submission; only a credential should")
	}
}

func TestAnUnwrittenCaseCannotBeSubmitted(t *testing.T) {
	p, l := fixture(t)
	tune(t, p, ".ilk/bin/demo.sh", "echo done", "echo done twice")

	prop, err := Build(p, l)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Draft(p, prop); err != nil {
		t.Fatal(err)
	}

	_, blockers, err := Ready(p, prop)
	if err != nil {
		t.Fatal(err)
	}
	// A maintainer receiving diffs with no case attached learns to ignore the
	// whole channel, which costs far more than any single proposal is worth.
	if len(blockers) < 2 {
		t.Fatalf("both judgement sections should block, got %v", blockers)
	}

	written := strings.ReplaceAll(prop.Render(), Marker, "Because:")
	mustWrite(t, p.Repo.Path(prop.Path()), written)
	_, blockers, err = Ready(p, prop)
	if err != nil {
		t.Fatal(err)
	}
	if len(blockers) != 0 {
		t.Fatalf("a written proposal is still blocked: %v", blockers)
	}
}

func TestDraftingTwiceDoesNotDiscardWhatSomebodyWrote(t *testing.T) {
	p, l := fixture(t)
	tune(t, p, ".ilk/bin/demo.sh", "echo done", "echo done twice")

	prop, err := Build(p, l)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Draft(p, prop); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, p.Repo.Path(prop.Path()), "the case, written by hand\n")

	if _, err := Draft(p, prop); err == nil {
		t.Fatal("the second draft overwrote the first")
	}
	if got := read(t, p, prop.Path()); got != "the case, written by hand\n" {
		t.Fatalf("the written case was lost: %q", got)
	}
}

func TestALayerThatDeclaresNoContributionHasNowhereToSendOne(t *testing.T) {
	p, l := fixture(t)
	l.Loaded.Manifest.Contribution = nil
	tune(t, p, ".ilk/bin/demo.sh", "echo done", "echo done twice")

	prop, err := Build(p, l)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Draft(p, prop); err != nil {
		t.Fatal(err)
	}
	_, blockers, err := Ready(p, prop)
	if err != nil {
		t.Fatal(err)
	}
	// No `contribution:` block is a layer saying it does not take proposals.
	if !containsSubstring(blockers, "has not opted in") {
		t.Fatalf("expected a refusal naming the missing block, got %v", blockers)
	}
}

func TestSubmittingMarksTheProposalOpen(t *testing.T) {
	// A queue full of things still called drafts tells a maintainer nothing about
	// what needs them.
	document := "---\nlayer: test/demo\nstatus: draft\n---\n\n# Body\n"
	got := opened(document)
	if !strings.Contains(got, "status: open") || strings.Contains(got, "status: draft") {
		t.Fatalf("status was not changed: %q", got)
	}
}

func TestGuidelinesComeFromTheLayer(t *testing.T) {
	_, l := fixture(t)
	// Standards a contributor only learns in review are standards that waste
	// everybody's time.
	text, ok := Guidelines(l)
	if !ok {
		t.Fatal("the layer ships guidelines and they were not found")
	}
	if !strings.Contains(text, "toolchain") {
		t.Fatalf("wrong file: %q", text)
	}
}

func TestHistoryDistinguishesAHabitFromAChangeOfMind(t *testing.T) {
	p, l := fixture(t)
	git(t, p.Repo.Root, "init", "-q")
	git(t, p.Repo.Root, "config", "user.email", "t@example.com")
	git(t, p.Repo.Root, "config", "user.name", "Test")
	git(t, p.Repo.Root, "add", "-A")
	git(t, p.Repo.Root, "commit", "-qm", "adopt")

	for i, replacement := range []string{"echo one", "echo two", "echo three"} {
		prev := "echo done"
		if i > 0 {
			prev = []string{"echo one", "echo two"}[i-1]
		}
		tune(t, p, ".ilk/bin/demo.sh", prev, replacement)
		git(t, p.Repo.Root, "add", "-A")
		git(t, p.Repo.Root, "commit", "-qm", "repair again")
	}

	prop, err := Build(p, l)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := edit(prop, ".ilk/bin/demo.sh")
	// One change is a change of mind. Four is a repository repeatedly repairing
	// the same thing, and upstream should be told which it is looking at.
	if e.Edits < 4 {
		t.Fatalf("counted %d changes to the file, want at least 4", e.Edits)
	}
	if !strings.Contains(e.HistoryPhrase(), "times") {
		t.Fatalf("the phrase does not convey repetition: %q", e.HistoryPhrase())
	}
}

// ---------------------------------------------------------------- helpers

func read(t *testing.T, p *engine.Project, rel string) string {
	t.Helper()
	data, err := os.ReadFile(p.Repo.Path(rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func containsSubstring(all []string, want string) bool {
	for _, s := range all {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// reapply re-plans and re-applies after the layer's source changed, which is what
// an upgrade does.
func reapply(t *testing.T, p *engine.Project) (*engine.Project, *engine.ResolvedLayer) {
	t.Helper()
	p2, err := engine.NewProject(p.Repo, p.Config, p.Lock, "test")
	if err != nil {
		t.Fatal(err)
	}
	pl, err := p2.Plan(engine.PlanOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := p2.Apply(pl); err != nil {
		t.Fatal(err)
	}
	p3, err := engine.NewProject(p.Repo, p.Config, mustLoadLock(t, p.Repo.Root), "test")
	if err != nil {
		t.Fatal(err)
	}
	return p3, p3.Layers[0]
}

// withTarget turns on agent targets, so skills are actually written to disk.
func withTarget(t *testing.T, p *engine.Project, names ...string) (*engine.Project, *engine.ResolvedLayer) {
	t.Helper()
	if len(names) == 0 {
		names = []string{"agents-md"}
	}
	p.Config.Targets = names
	return reapply(t, p)
}

var (
	_ = layer.Loaded{}
	_ = manifest.Layer{}
	_ = render.Context{}
	_ = basestore.DirName
)
