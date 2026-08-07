package checks

import (
	"path/filepath"
	"testing"

	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/repo"
)

// A placeholder is the one defect nothing structural catches: the heading is
// present, the frontmatter is valid, and only the words give it away.

func forbidProject(t *testing.T) *engine.Project {
	t.Helper()
	root := t.TempDir()
	p, err := engine.NewProject(&repo.Repo{Root: root}, config.Default(), lock.New(), "test")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAnUnwrittenPlaceholderIsAFinding(t *testing.T) {
	p := forbidProject(t)
	writeFile(t, p.Repo.Root, "proposals/one.md", "---\nstatus: open\n---\n\nbody\nTODO(you): the case\n")
	writeFile(t, p.Repo.Root, "proposals/two.md", "---\nstatus: open\n---\n\nfully written\n")

	findings, err := checkForbid(p, map[string]any{
		"dirs": []any{"proposals"},
		"patterns": []any{map[string]any{
			"text": "TODO(you):", "reason": "the case is still unwritten",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
	}
	if findings[0].Path != filepath.Join("proposals", "one.md") {
		t.Fatalf("wrong document: %s", findings[0].Path)
	}
	if findings[0].Line != 6 {
		t.Fatalf("line %d, want 6 — a finding nobody can locate is half a finding", findings[0].Line)
	}
	if findings[0].Message != "the case is still unwritten" {
		t.Fatalf("the finding repeats the pattern instead of saying what to do: %q", findings[0].Message)
	}
}

func TestAPatternWithNoReasonIsRejected(t *testing.T) {
	p := forbidProject(t)
	// A finding that only says what matched tells a reader nothing about what to
	// do, which is the failure this whole check vocabulary exists to avoid.
	_, err := checkForbid(p, map[string]any{
		"dirs":     []any{"proposals"},
		"patterns": []any{map[string]any{"text": "TODO"}},
	})
	if err == nil {
		t.Fatal("a pattern with no reason was accepted")
	}
}

func TestForbidRespectsTheDocumentsState(t *testing.T) {
	p := forbidProject(t)
	writeFile(t, p.Repo.Root, "proposals/draft.md", "---\nstatus: draft\n---\n\nTODO(you): still writing\n")

	findings, err := checkForbid(p, map[string]any{
		"dirs":     []any{"proposals"},
		"where":    map[string]any{"status": "open"},
		"patterns": []any{map[string]any{"text": "TODO(you):", "reason": "unwritten"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// A document still declaring itself a draft has not made any claim to be
	// finished, so holding it to a finished document's standard is noise.
	if len(findings) != 0 {
		t.Fatalf("a draft was held to the finished standard: %+v", findings)
	}
}
