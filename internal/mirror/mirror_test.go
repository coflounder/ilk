package mirror

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/coflounder/ilk/internal/repo"
)

// A write to somebody else's system is either correct or it does not happen.
// These tests hold that line with a fake provider, so the identity, diffing and
// refusal logic is verified without a network or an account — which is also the
// only way it can be verified in CI.

// fixture builds a repository with plan documents and a provider that answers
// from a fixed JSON file.
func fixture(t *testing.T, remoteJSON string) (*engine.Project, *engine.ResolvedLayer, manifest.Mirror) {
	t.Helper()
	root := t.TempDir()

	mustWrite(t, filepath.Join(root, ".ilk", "config.yaml"), "version: 1\ntargets: []\nlayers: []\n")
	// The record layer creates this; a mirror pointing at a directory that does
	// not exist is a misconfiguration and says so, rather than silently
	// mirroring nothing.
	if err := os.MkdirAll(filepath.Join(root, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "remote.json"), remoteJSON)

	// The provider: list cats the fixture, create prints an id, update records
	// that it was asked. Nothing here reaches a network.
	mustWrite(t, filepath.Join(root, "list.sh"), "#!/bin/sh\ncat remote.json\n")
	mustWrite(t, filepath.Join(root, "create.sh"),
		"#!/bin/sh\ncat >> created.jsonl\necho REMOTE-NEW\n")
	mustWrite(t, filepath.Join(root, "update.sh"),
		"#!/bin/sh\ncat >> updated.jsonl\n")
	for _, name := range []string{"list.sh", "create.sh", "update.sh"} {
		if err := os.Chmod(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	cfg := config.Default()
	cfg.Targets = nil
	p, err := engine.NewProject(&repo.Repo{Root: root}, cfg, lock.New(), "test")
	if err != nil {
		t.Fatal(err)
	}

	l := &engine.ResolvedLayer{
		Loaded: &layerStub,
		Vars:   map[string]string{},
	}
	l.Ctx.Vars = map[string]string{}

	m := manifest.Mirror{
		ID: "fake", Summary: "Fake tracker", Dir: "plans", Match: "^SPEC-",
		Key: "tracker", List: "sh list.sh", Create: "sh create.sh", Update: "sh update.sh",
	}
	return p, l, m
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

func spec(t *testing.T, p *engine.Project, name, frontmatter string) {
	t.Helper()
	mustWrite(t, filepath.Join(p.Repo.Root, "plans", name), "---\n"+frontmatter+"---\n\n# Body\n")
}

func read(t *testing.T, p *engine.Project, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(p.Repo.Root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func ops(plan *Plan) map[Op]int {
	out := map[Op]int{}
	for _, a := range plan.Actions {
		out[a.Op]++
	}
	return out
}

func TestUnlinkedDocumentIsCreated(t *testing.T) {
	p, l, m := fixture(t, `[]`)
	spec(t, p, "SPEC-one.md", "id: spec-one\ntitle: First thing\nstatus: active\n")

	plan, err := Build(p, l, m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := ops(plan); got[OpCreate] != 1 {
		t.Fatalf("expected one create, got %v", got)
	}
}

func TestCreatingRecordsTheRemoteIdInTheOwnedKey(t *testing.T) {
	p, l, m := fixture(t, `[]`)
	spec(t, p, "SPEC-one.md", "id: spec-one\ntitle: First thing\nstatus: active\nowner: someone\n")

	plan, _ := Build(p, l, m, Options{})
	result, err := Apply(p, l, m, plan, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || len(result.Errors) > 0 {
		t.Fatalf("expected one create, got %+v", result)
	}

	got := read(t, p, "plans/SPEC-one.md")
	if !strings.Contains(got, "tracker:") || !strings.Contains(got, "REMOTE-NEW") {
		t.Fatalf("the remote id was not recorded:\n%s", got)
	}
	// Everything the human wrote must be exactly where they left it.
	for _, want := range []string{"id: spec-one", "title: First thing", "status: active", "owner: someone", "# Body"} {
		if !strings.Contains(got, want) {
			t.Errorf("writing the id disturbed the document; %q missing:\n%s", want, got)
		}
	}
}

func TestASecondApplyIsANoOp(t *testing.T) {
	p, l, m := fixture(t, `[]`)
	spec(t, p, "SPEC-one.md", "id: spec-one\ntitle: First thing\nstatus: active\n")

	plan, _ := Build(p, l, m, Options{})
	if _, err := Apply(p, l, m, plan, Options{}); err != nil {
		t.Fatal(err)
	}

	// The tracker now holds what was created.
	mustWrite(t, filepath.Join(p.Repo.Root, "remote.json"),
		`[{"id":"REMOTE-NEW","title":"First thing","status":"active"}]`)

	again, err := Build(p, l, m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !again.Empty() {
		t.Fatalf("a second plan wanted to write: %+v", again.Writes())
	}
}

func TestDivergentFieldsBecomeAnUpdate(t *testing.T) {
	p, l, m := fixture(t, `[{"id":"R1","title":"Old title","status":"proposed"}]`)
	spec(t, p, "SPEC-one.md", "id: spec-one\ntitle: New title\nstatus: active\ntracker:\n  id: R1\n")

	plan, err := Build(p, l, m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	writes := plan.Writes()
	if len(writes) != 1 || writes[0].Op != OpUpdate {
		t.Fatalf("expected one update, got %+v", plan.Actions)
	}
	if len(writes[0].Changes) != 2 {
		t.Fatalf("both title and status differ, got %+v", writes[0].Changes)
	}
}

// The record is the source of truth. A tracker somebody edited by hand is a
// change to push, never an update to pull — otherwise "which one is right"
// becomes a question somebody has to answer every time.
func TestTheRecordWinsOverTheTracker(t *testing.T) {
	p, l, m := fixture(t, `[{"id":"R1","title":"Edited in the tracker","status":"done"}]`)
	spec(t, p, "SPEC-one.md", "id: spec-one\ntitle: The real title\nstatus: active\ntracker:\n  id: R1\n")

	plan, _ := Build(p, l, m, Options{})
	writes := plan.Writes()
	if len(writes) != 1 {
		t.Fatalf("expected one update, got %+v", plan.Actions)
	}
	for _, c := range writes[0].Changes {
		if c.Field == "title" && c.Record != "The real title" {
			t.Errorf("the record's title must be the one pushed, got %q", c.Record)
		}
	}
	if before := read(t, p, "plans/SPEC-one.md"); !strings.Contains(before, "The real title") {
		t.Error("planning must not rewrite the document")
	}
}

// A wrong link is silent and permanent: every later sync writes to the wrong
// item, and nobody finds out until they read the tracker and do not recognise it.
func TestAmbiguousTitlesAreRefusedAndNamed(t *testing.T) {
	p, l, m := fixture(t, `[
		{"id":"R1","title":"Fix the thing","url":"https://example.test/1"},
		{"id":"R2","title":"fix the  thing","url":"https://example.test/2"}
	]`)
	spec(t, p, "SPEC-one.md", "id: spec-one\ntitle: Fix the thing\nstatus: active\n")

	plan, err := Build(p, l, m, Options{Link: true})
	if err != nil {
		t.Fatal(err)
	}
	blocked := plan.Blocked()
	if len(blocked) != 1 {
		t.Fatalf("expected the document to be refused, got %+v", plan.Actions)
	}
	if len(blocked[0].Candidates) != 2 {
		t.Fatalf("the refusal must name every candidate, got %+v", blocked[0].Candidates)
	}
	if len(plan.Writes()) != 0 {
		t.Fatalf("an ambiguous document must not be written, got %+v", plan.Writes())
	}
}

func TestAmbiguityDoesNotBlockTheRestOfTheRun(t *testing.T) {
	p, l, m := fixture(t, `[
		{"id":"R1","title":"Shared"},
		{"id":"R2","title":"Shared"}
	]`)
	spec(t, p, "SPEC-ambiguous.md", "id: spec-a\ntitle: Shared\nstatus: active\n")
	spec(t, p, "SPEC-clear.md", "id: spec-b\ntitle: Quite distinct\nstatus: active\n")

	plan, _ := Build(p, l, m, Options{Link: true})
	result, err := Apply(p, l, m, plan, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 {
		t.Fatalf("the unambiguous document should still have been created, got %+v", result)
	}
	if got := read(t, p, "plans/SPEC-ambiguous.md"); strings.Contains(got, "tracker:") {
		t.Error("the ambiguous document must not have been linked")
	}
}

func TestLinkingMatchesExistingItemsByTitle(t *testing.T) {
	p, l, m := fixture(t, `[{"id":"R7","title":"Already in the tracker","url":"https://example.test/7"}]`)
	spec(t, p, "SPEC-one.md", "id: spec-one\ntitle: Already in the tracker\nstatus: active\n")

	plan, err := Build(p, l, m, Options{Link: true})
	if err != nil {
		t.Fatal(err)
	}
	writes := plan.Writes()
	if len(writes) != 1 || writes[0].Op != OpLink {
		t.Fatalf("expected a link, got %+v", plan.Actions)
	}

	if _, err := Apply(p, l, m, plan, Options{}); err != nil {
		t.Fatal(err)
	}
	got := read(t, p, "plans/SPEC-one.md")
	if !strings.Contains(got, "R7") || !strings.Contains(got, "example.test/7") {
		t.Fatalf("the link should record both id and url:\n%s", got)
	}
}

// Without --link, an unlinked document is created rather than guessed at. Title
// matching is the adoption path, and adopting is a different act from syncing.
func TestWithoutLinkTitlesAreNotMatched(t *testing.T) {
	p, l, m := fixture(t, `[{"id":"R7","title":"Already in the tracker"}]`)
	spec(t, p, "SPEC-one.md", "id: spec-one\ntitle: Already in the tracker\nstatus: active\n")

	plan, _ := Build(p, l, m, Options{})
	if got := ops(plan); got[OpCreate] != 1 || got[OpLink] != 0 {
		t.Fatalf("expected a create and no link, got %v", got)
	}
}

func TestAVanishedRemoteItemIsReportedNotRecreated(t *testing.T) {
	p, l, m := fixture(t, `[]`)
	spec(t, p, "SPEC-one.md", "id: spec-one\ntitle: Gone\nstatus: active\ntracker:\n  id: R-DELETED\n")

	plan, err := Build(p, l, m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := ops(plan); got[OpOrphan] != 1 || got[OpCreate] != 0 {
		t.Fatalf("expected an orphan report and no create, got %v", got)
	}
	if !plan.Empty() {
		t.Fatal("an orphan must not become a write")
	}
}

// ilk never deletes in somebody else's system. Deciding an item is dead is not
// its call to make.
func TestAnUntrackedRemoteItemIsReportedNeverDeleted(t *testing.T) {
	p, l, m := fixture(t, `[{"id":"R9","title":"Somebody else's work"}]`)

	plan, err := Build(p, l, m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := ops(plan); got[OpUntracked] != 1 {
		t.Fatalf("expected the item to be reported, got %v", got)
	}
	if !plan.Empty() {
		t.Fatal("an untracked item must never produce a write")
	}
}

func TestPlanningWritesNothingAnywhere(t *testing.T) {
	p, l, m := fixture(t, `[{"id":"R1","title":"Old","status":"proposed"}]`)
	spec(t, p, "SPEC-one.md", "id: spec-one\ntitle: New\nstatus: active\ntracker:\n  id: R1\n")
	before := read(t, p, "plans/SPEC-one.md")

	if _, err := Build(p, l, m, Options{Link: true}); err != nil {
		t.Fatal(err)
	}
	if after := read(t, p, "plans/SPEC-one.md"); after != before {
		t.Error("planning modified a document")
	}
	for _, name := range []string{"created.jsonl", "updated.jsonl"} {
		if _, err := os.Stat(filepath.Join(p.Repo.Root, name)); err == nil {
			t.Errorf("planning called the provider's %s command", name)
		}
	}
}

func TestAProviderFailureStopsOneDocumentNotTheRun(t *testing.T) {
	p, l, m := fixture(t, `[]`)
	// A create command that refuses everything.
	mustWrite(t, filepath.Join(p.Repo.Root, "create.sh"), "#!/bin/sh\ncat >/dev/null\nexit 1\n")
	if err := os.Chmod(filepath.Join(p.Repo.Root, "create.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec(t, p, "SPEC-one.md", "id: spec-one\ntitle: One\nstatus: active\n")
	spec(t, p, "SPEC-two.md", "id: spec-two\ntitle: Two\nstatus: active\n")

	plan, _ := Build(p, l, m, Options{})
	result, err := Apply(p, l, m, plan, Options{})
	if err != nil {
		t.Fatalf("a provider failure should be reported, not returned as a run error: %v", err)
	}
	if len(result.Errors) != 2 {
		t.Fatalf("both documents should have reported a failure, got %+v", result)
	}
	if result.Created != 0 {
		t.Error("nothing was created, so nothing should be counted")
	}
	if got := read(t, p, "plans/SPEC-one.md"); strings.Contains(got, "tracker:") {
		t.Error("a failed create must not record an id")
	}
}

func TestAMalformedProviderResponseIsAnActionableError(t *testing.T) {
	p, l, m := fixture(t, `not json at all`)
	spec(t, p, "SPEC-one.md", "id: spec-one\ntitle: One\nstatus: active\n")

	_, err := Build(p, l, m, Options{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "JSON array") {
		t.Errorf("the error should say what shape was expected, got %q", err)
	}
}

func TestARemoteItemWithNoIdIsRefused(t *testing.T) {
	p, l, m := fixture(t, `[{"title":"No id here"}]`)

	_, err := Build(p, l, m, Options{})
	if err == nil || !strings.Contains(err.Error(), "no id") {
		t.Fatalf("an item with no id cannot be matched to anything; got %v", err)
	}
}

func TestRewritingTheKeyReplacesRatherThanDuplicates(t *testing.T) {
	p, _, m := fixture(t, `[]`)
	spec(t, p, "SPEC-one.md", "id: spec-one\ntitle: One\nstatus: active\ntracker:\n  id: OLD\n  url: https://old.test\n")

	if err := writeRemoteID(p, m, "plans/SPEC-one.md", "NEW", "https://new.test"); err != nil {
		t.Fatal(err)
	}
	got := read(t, p, "plans/SPEC-one.md")
	if strings.Contains(got, "OLD") || strings.Contains(got, "old.test") {
		t.Errorf("the previous value survived:\n%s", got)
	}
	if strings.Count(got, "tracker:") != 1 {
		t.Errorf("the key was duplicated:\n%s", got)
	}
	if !strings.Contains(got, "title: One") {
		t.Errorf("neighbouring keys were disturbed:\n%s", got)
	}
}

// layerStub is a minimal loaded layer: mirrors need a context to render through,
// and nothing here exercises templating.
var layerStub = stubLoaded()

// A mirror pointing at a directory that does not exist is a misconfiguration, not
// an empty tracker. Silently mirroring nothing is the same trap as a `covers:`
// pattern that matches nothing: it looks like it is working.
func TestAMissingDirectoryIsAnError(t *testing.T) {
	p, l, m := fixture(t, `[]`)
	m.Dir = "plns"

	_, err := Build(p, l, m, Options{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "plns") {
		t.Errorf("the error should name the directory it could not read, got %q", err)
	}
}

// The three tests below all guard the same failure shape: a mirror that appears
// to be working while quietly leaving something out.

func TestTheStatusReachesTheProviderOnCreate(t *testing.T) {
	p, l, m := fixture(t, `[]`)
	spec(t, p, "SPEC-one.md", "id: spec-one\ntitle: First thing\nstatus: in progress\n")
	// A create has nothing to diff against, so the status cannot be recovered from
	// the change list. Losing it here puts an item on the board with no status and
	// makes the next run go back and fix what this one should have got right.
	mustWrite(t, filepath.Join(p.Repo.Root, "create.sh"),
		"#!/bin/sh\necho \"$ILK_MIRROR_STATUS\" > create-status\necho REMOTE-NEW\n")

	plan, err := Build(p, l, m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(p, l, m, plan, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(read(t, p, "create-status")); got != "in progress" {
		t.Fatalf("the provider saw status %q, want %q", got, "in progress")
	}
}

func TestMatchIsRenderedThroughTheLayersVariables(t *testing.T) {
	p, l, m := fixture(t, `[]`)
	l.Ctx.Vars = map[string]string{"match": "^SPEC-"}
	m.Match = "{{ .Vars.match }}"
	spec(t, p, "SPEC-one.md", "id: spec-one\ntitle: First thing\n")
	spec(t, p, "NOTE-aside.md", "id: note\ntitle: Not a spec\n")

	plan, err := Build(p, l, m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// An unrendered pattern compiles to a regex matching nothing, and a mirror
	// with no documents is indistinguishable from one with nothing to do.
	if got := ops(plan); got[OpCreate] != 1 {
		t.Fatalf("expected the pattern to select exactly the one spec, got %v", got)
	}
	if plan.Actions[0].Path != "plans/SPEC-one.md" {
		t.Fatalf("selected %s", plan.Actions[0].Path)
	}
}

func TestADocumentWithUnreadableFrontmatterIsNamed(t *testing.T) {
	p, l, m := fixture(t, `[]`)
	spec(t, p, "SPEC-good.md", "id: good\ntitle: Fine\n")
	mustWrite(t, filepath.Join(p.Repo.Root, "plans", "SPEC-bad.md"), "title: no fence\n\n# Body\n")

	// Skipping it would take the document off the tracker without saying so.
	_, err := Build(p, l, m, Options{})
	if err == nil {
		t.Fatal("expected an error naming the unreadable document")
	}
	if !strings.Contains(err.Error(), "SPEC-bad.md") {
		t.Fatalf("the error does not name the document: %v", err)
	}
	if !strings.Contains(err.Error(), "fix:") {
		t.Fatalf("the error carries no fix: %v", err)
	}
}
