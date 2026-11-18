package checks

import (
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/coflounder/ilk/internal/repo"
)

// A script check's exit code is a contract: 0 pass, 1 fail, anything else
// error. Before it, a missing script (127) read as "your repository is wrong".

func runScript(t *testing.T, check manifest.Check) Result {
	t.Helper()
	p, err := engine.NewProject(&repo.Repo{Root: t.TempDir()}, config.Default(), lock.New(), "test")
	if err != nil {
		t.Fatal(err)
	}
	return runOne(p, registered{check: check, layer: "test/fake"}, map[string]string{}, Options{})
}

func TestScriptExitCodesMapToPassFailAndError(t *testing.T) {
	for _, tc := range []struct {
		run    string
		status Status
		reason string
	}{
		{"exit 0", StatusPass, ""},
		{"exit 1", StatusFail, ""},
		{"exit 2", StatusError, "exited 2"},
		{"./no-such-script.sh", StatusError, "127"},
	} {
		res := runScript(t, manifest.Check{ID: "fake.x", Run: tc.run, Fix: "x"})
		if res.Status != tc.status {
			t.Errorf("%q: status %s, want %s (%s)", tc.run, res.Status, tc.status, res.Reason)
		}
		if !strings.Contains(res.Reason, tc.reason) {
			t.Errorf("%q: reason %q should mention %q", tc.run, res.Reason, tc.reason)
		}
	}
}

func TestJSONLFindingsAreReadAndABadLineIsOnlyAWarning(t *testing.T) {
	script := `printf '%s\n' '{"path":"a.md","line":3,"message":"no owner"}' 'not json' '{"path":"b.md"}' '{"message":"global"}'; echo "detail" >&2; exit 1`
	res := runScript(t, manifest.Check{ID: "fake.x", Run: script, Findings: "jsonl", Fix: "x"})
	if res.Status != StatusFail {
		t.Fatalf("status %s, want fail: %s", res.Status, res.Reason)
	}
	want := []Finding{{Path: "a.md", Line: 3, Message: "no owner"}, {Message: "global"}}
	if len(res.Findings) != len(want) {
		t.Fatalf("findings %+v, want %+v", res.Findings, want)
	}
	for i := range want {
		if res.Findings[i] != want[i] {
			t.Errorf("finding %d = %+v, want %+v", i, res.Findings[i], want[i])
		}
	}
	if len(res.Warnings) != 2 || !strings.Contains(res.Warnings[0], "line 2") || !strings.Contains(res.Warnings[1], "line 3") {
		t.Errorf("each unreadable line should be one warning: %v", res.Warnings)
	}
	if res.Output != "detail" {
		t.Errorf("stderr should be kept as output, got %q", res.Output)
	}
}

func TestOnlySelectsAWholeLayerByName(t *testing.T) {
	all := []registered{
		{check: manifest.Check{ID: "pr-prep.a"}},
		{check: manifest.Check{ID: "pr-prep.b"}},
		{check: manifest.Check{ID: "pr-prepx.c"}},
		{check: manifest.Check{ID: "record.d"}},
	}
	got, err := filter(all, Options{Only: []string{"pr-prep"}})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range got {
		ids = append(ids, r.check.ID)
	}
	if strings.Join(ids, ",") != "pr-prep.a,pr-prep.b" {
		t.Errorf("--only pr-prep selected %v", ids)
	}
	if _, err := filter(all, Options{Only: []string{"nope"}}); err == nil {
		t.Error("an unknown name should still be an error")
	}
}
