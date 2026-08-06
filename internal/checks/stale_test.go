package checks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/repo"
)

// Staleness is measured against what a document says it covers, not against the
// calendar. These tests defend that: an old document about frozen code is fine, a
// fresh document about churning code is not, and a document that cannot be checked
// says so rather than passing silently.

func gitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")

	writeFile(t, root, "src/payments/pay.go", "package payments\n")
	writeFile(t, root, "src/legacy/old.go", "package legacy\n")
	run("add", "-A")
	run("commit", "-qm", "init")
	return root
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitRun(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// churn makes n commits to a path, simulating an active subsystem.
func churn(t *testing.T, root, rel string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		path := filepath.Join(root, rel)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(f, "// change %d\n", i)
		f.Close()
		gitRun(t, root, "commit", "-qam", fmt.Sprintf("change %d", i))
	}
}

// project builds a minimal Project rooted at an existing git repository.
func project(t *testing.T, root string) *engine.Project {
	t.Helper()
	cfg := config.Default()
	cfg.Targets = nil
	p, err := engine.NewProject(&repo.Repo{Root: root}, cfg, lock.New(), "test")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func staleArgs() map[string]any {
	return map[string]any{
		"dirs":                 []any{"docs"},
		"review_after_commits": "10",
		"exempt":               []any{"README.md"},
	}
}

func TestFrozenCodeNeverMakesItsDocumentStale(t *testing.T) {
	root := gitRepo(t)
	writeFile(t, root, "docs/ARCH-legacy.md", `---
id: arch-legacy
title: Legacy
status: current
updated: 2015-01-01
covers:
  - src/legacy/**
---
# Legacy
`)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-qm", "add legacy doc")

	// Ten years old by its own account, and a busy repository around it — but
	// nothing has touched the code it describes.
	churn(t, root, "src/payments/pay.go", 30)

	findings, err := checkStale(project(t, root), staleArgs())
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("a document about untouched code is not stale, however old: %+v", findings)
	}
}

func TestChurningCodeMakesItsDocumentStale(t *testing.T) {
	root := gitRepo(t)
	writeFile(t, root, "docs/ARCH-payments.md", `---
id: arch-payments
title: Payments
status: current
updated: 2026-01-01
covers:
  - src/payments/**
---
# Payments
`)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-qm", "add payments doc")

	churn(t, root, "src/payments/pay.go", 12)

	findings, err := checkStale(project(t, root), staleArgs())
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected the payments doc to be stale, got %+v", findings)
	}
	msg := findings[0].Message
	for _, want := range []string{"src/payments/**", "commits touched", "most recent"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the finding should say what changed; %q missing %q", msg, want)
		}
	}
}

func TestChurnBelowTheThresholdIsNotStale(t *testing.T) {
	root := gitRepo(t)
	writeFile(t, root, "docs/ARCH-payments.md", `---
id: arch-payments
title: Payments
status: current
updated: 2026-01-01
covers:
  - src/payments/**
---
# Payments
`)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-qm", "add payments doc")

	churn(t, root, "src/payments/pay.go", 3)

	findings, err := checkStale(project(t, root), staleArgs())
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("three commits is below the threshold of ten: %+v", findings)
	}
}

func TestADocumentCanTightenItsOwnThreshold(t *testing.T) {
	root := gitRepo(t)
	// A volatile document that wants re-reading after any two commits, in a
	// project whose default is ten.
	writeFile(t, root, "docs/ARCH-payments.md", `---
id: arch-payments
title: Payments
status: current
updated: 2026-01-01
review_after_commits: 2
covers:
  - src/payments/**
---
# Payments
`)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-qm", "add payments doc")

	churn(t, root, "src/payments/pay.go", 4)

	findings, err := checkStale(project(t, root), staleArgs())
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("the document's own threshold should win over the project default: %+v", findings)
	}
}

func TestReviewingTheDocumentClearsIt(t *testing.T) {
	root := gitRepo(t)
	writeFile(t, root, "docs/ARCH-payments.md", `---
id: arch-payments
title: Payments
status: current
updated: 2026-01-01
covers:
  - src/payments/**
---
# Payments
`)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-qm", "add payments doc")
	churn(t, root, "src/payments/pay.go", 12)

	if findings, _ := checkStale(project(t, root), staleArgs()); len(findings) != 1 {
		t.Fatalf("setup: expected the document to be stale, got %+v", findings)
	}

	// Committing a change to the document is the acknowledgement.
	writeFile(t, root, "docs/ARCH-payments.md", `---
id: arch-payments
title: Payments
status: current
updated: 2026-08-06
covers:
  - src/payments/**
---
# Payments

Re-read against the current code.
`)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-qm", "review payments doc")

	findings, err := checkStale(project(t, root), staleArgs())
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("reviewing the document should settle it: %+v", findings)
	}
}

func TestADocumentWithNoCoverageIsReported(t *testing.T) {
	root := gitRepo(t)
	writeFile(t, root, "docs/ARCH-mystery.md", `---
id: arch-mystery
title: Mystery
status: current
updated: 2026-01-01
---
# Mystery
`)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-qm", "add doc")

	p := project(t, root)
	args := map[string]any{"dirs": []any{"docs"}, "exempt": []any{"README.md"}}
	findings, err := checkCoverage(p, args)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "no `covers:`") {
		t.Fatalf("a document that can never go stale should be reported: %+v", findings)
	}

	// And the staleness check stays quiet about it, rather than saying the same
	// thing twice in different words.
	if stale, _ := checkStale(p, staleArgs()); len(stale) != 0 {
		t.Errorf("record.stale should defer to record.coverage here: %+v", stale)
	}
}

// A pattern that matches nothing is the worst case: the document looks governed
// and is not. This is the trap the coverage check mainly exists to catch.
func TestACoversPatternMatchingNothingIsReported(t *testing.T) {
	root := gitRepo(t)
	writeFile(t, root, "docs/ARCH-typo.md", `---
id: arch-typo
title: Typo
status: current
updated: 2026-01-01
covers:
  - src/paymnets/**
---
# Typo
`)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-qm", "add doc")

	findings, err := checkCoverage(project(t, root), map[string]any{
		"dirs": []any{"docs"}, "exempt": []any{"README.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "matches no tracked file") {
		t.Fatalf("a typo in a glob silently exempts the document; it must be caught: %+v", findings)
	}
}

func TestAbsoluteAgeLimitIsOffUnlessAskedFor(t *testing.T) {
	root := gitRepo(t)
	writeFile(t, root, "docs/ARCH-legacy.md", `---
id: arch-legacy
title: Legacy
status: current
updated: 2015-01-01
covers:
  - src/legacy/**
---
# Legacy
`)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-qm", "add doc")

	p := project(t, root)
	if findings, _ := checkStale(p, staleArgs()); len(findings) != 0 {
		t.Fatalf("no age limit is configured, so age alone must not fail: %+v", findings)
	}

	// Opting in is how a project says "some of our documents go stale because the
	// world moved, not because our code did".
	args := staleArgs()
	args["max_age_days"] = "30"
	findings, err := checkStale(p, args)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "regardless of code changes") {
		t.Fatalf("with max_age_days set, an ancient document should be flagged: %+v", findings)
	}
}

func TestWithoutGitNothingIsClaimed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/ARCH-x.md", `---
id: arch-x
title: X
status: current
updated: 2015-01-01
covers:
  - src/**
---
# X
`)
	findings, err := checkStale(project(t, root), staleArgs())
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("with no history there is nothing to measure against: %+v", findings)
	}
}

// Not every document describes code. A rationale, a glossary or a convention has
// no path to be coupled to, and forcing a fake one would be worse than useless.
// The distinction that matters is between a decision and an oversight.
func TestAnExplicitlyUncoupledDocumentIsAccepted(t *testing.T) {
	root := gitRepo(t)
	writeFile(t, root, "docs/REF-rationale.md", `---
id: ref-rationale
title: Why we chose this
status: current
updated: 2015-01-01
covers: []
---
# Why we chose this
`)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-qm", "add doc")
	churn(t, root, "src/payments/pay.go", 30)

	p := project(t, root)
	coverage, err := checkCoverage(p, map[string]any{"dirs": []any{"docs"}, "exempt": []any{"README.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(coverage) != 0 {
		t.Fatalf("an explicit empty covers is a declaration, not an oversight: %+v", coverage)
	}
	if stale, _ := checkStale(p, staleArgs()); len(stale) != 0 {
		t.Fatalf("an uncoupled document cannot go stale: %+v", stale)
	}
}

// The distinction is only worth having if the two cases really do differ.
func TestMissingAndEmptyCoversAreDifferent(t *testing.T) {
	root := gitRepo(t)
	writeFile(t, root, "docs/ARCH-missing.md", "---\nid: a\ntitle: A\nstatus: current\nupdated: 2026-01-01\n---\n# A\n")
	writeFile(t, root, "docs/ARCH-empty.md", "---\nid: b\ntitle: B\nstatus: current\nupdated: 2026-01-01\ncovers: []\n---\n# B\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-qm", "add docs")

	findings, err := checkCoverage(project(t, root), map[string]any{"dirs": []any{"docs"}, "exempt": []any{"README.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Path != "docs/ARCH-missing.md" {
		t.Fatalf("only the document that never decided should be reported: %+v", findings)
	}
}
