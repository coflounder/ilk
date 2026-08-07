package checks

import (
	"testing"

	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/coflounder/ilk/internal/repo"
)

// Some artifacts ilk owns can never be in a fresh checkout: a git hook lives
// under .git, which git does not track, and anything inside an ignored directory
// is ignored on purpose — `scratch/` is declared that way by the record layer
// itself.
//
// Reporting their absence as drift made `ilk check` fail in CI for every
// repository, on files nobody could have restored because they were never
// committed. It passed on developer machines, where the files are present, which
// is why it survived to the first pull request.
func TestDriftIgnoresArtifactsGitDoesNotCarry(t *testing.T) {
	root := gitRepo(t)
	writeFile(t, root, ".gitignore", "scratch/\n")

	lk := lock.New()
	lk.Put(lock.Owner{
		ID:   "ilk/record",
		Kind: lock.KindLayer,
		Files: []lock.File{
			{Path: "scratch/.gitkeep", Mode: manifest.ModeManaged, Hash: lock.Hash(""), Owner: "ilk/record"},
			{Path: "docs/reference/README.md", Mode: manifest.ModeManaged, Hash: lock.Hash("hello\n"), Owner: "ilk/record"},
		},
	})
	lk.Put(lock.Owner{
		ID:   "target:git-hooks",
		Kind: lock.KindTarget,
		Files: []lock.File{
			{Path: ".git/hooks/pre-commit", Mode: manifest.ModeManaged, Hash: lock.Hash("#!/bin/sh\n"), Owner: "target:git-hooks"},
		},
	})

	// Every one of these is absent, as it would be in a clone. Only the tracked
	// document is a genuine finding.
	p, err := engine.NewProject(&repo.Repo{Root: root}, config.Default(), lk, "test")
	if err != nil {
		t.Fatal(err)
	}
	findings, err := checkDrift(p, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(findings) != 1 {
		t.Fatalf("expected only the tracked file to be reported, got %d: %+v", len(findings), findings)
	}
	if findings[0].Path != "docs/reference/README.md" {
		t.Errorf("reported %q; the gitignored and .git paths should be silent", findings[0].Path)
	}
}

// The exemption is about what git carries, not about missing files in general.
// A tracked artifact somebody deleted is still drift, or `ilk check` would go
// quiet exactly when ilk's output has been thrown away.
func TestDriftStillReportsATrackedFileThatWasDeleted(t *testing.T) {
	root := gitRepo(t)
	writeFile(t, root, ".gitignore", "scratch/\n")

	lk := lock.New()
	lk.Put(lock.Owner{
		ID:    "ilk/record",
		Kind:  lock.KindLayer,
		Files: []lock.File{{Path: "docs/reference/ARCH-x.md", Mode: manifest.ModeManaged, Hash: lock.Hash("x\n"), Owner: "ilk/record"}},
	})

	p, err := engine.NewProject(&repo.Repo{Root: root}, config.Default(), lk, "test")
	if err != nil {
		t.Fatal(err)
	}
	findings, err := checkDrift(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Path != "docs/reference/ARCH-x.md" {
		t.Fatalf("a deleted tracked artifact must still be drift, got %+v", findings)
	}
}

// Outside a git repository there is nothing to ask, and inventing an exemption
// would be as wrong as inventing a failure.
func TestDriftOutsideAGitRepositoryStillReports(t *testing.T) {
	root := t.TempDir()
	lk := lock.New()
	lk.Put(lock.Owner{
		ID:    "ilk/record",
		Kind:  lock.KindLayer,
		Files: []lock.File{{Path: "docs/reference/ARCH-x.md", Mode: manifest.ModeManaged, Hash: lock.Hash("x\n"), Owner: "ilk/record"}},
	})
	p, err := engine.NewProject(&repo.Repo{Root: root}, config.Default(), lk, "test")
	if err != nil {
		t.Fatal(err)
	}
	findings, err := checkDrift(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected the missing artifact to be reported, got %+v", findings)
	}
}
