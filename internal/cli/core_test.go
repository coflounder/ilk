package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The core tier — record and toolkit — is in every repository. It is implied
// rather than listed, cannot be added or removed, and search leaves it out.

func errorOf(t *testing.T, stderr string) (code, fix string) {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
			Fix  string `json:"fix"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &payload); err != nil {
		t.Fatalf("stderr is not a JSON error: %v\n%s", err, stderr)
	}
	return payload.Error.Code, payload.Error.Fix
}

func TestInitAddsTheDefaultTierAndImpliesTheCore(t *testing.T) {
	root := t.TempDir()
	if code, _, stderr := invoke(t, "-C", root, "init", "--yes", "--test-command", "true"); code != exitOK {
		t.Fatalf("init: exit %d\n%s", code, stderr)
	}
	if got, want := layersIn(t, root), []string{"ilk/compound-lessons", "ilk/gates", "ilk/secrets"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("the config lists %v, want the default tier %v and nothing implied", got, want)
	}
	for _, rel := range []string{
		"docs/reference/README.md",                  // record
		".agents/skills/add-a-layer/SKILL.md",       // toolkit
		".ilk/bin/ilk-secrets-scan.sh",              // secrets
		".agents/skills/compound-a-lesson/SKILL.md", // compound-lessons
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("%s should be on disk after init: %v", rel, err)
		}
	}
	if code, stdout, stderr := invoke(t, "-C", root, "check"); code != exitOK {
		t.Fatalf("a fresh init should check clean: exit %d\n%s%s", code, stdout, stderr)
	}
}

func TestInitWithoutATestCommandSkipsGatesAndSaysSo(t *testing.T) {
	root := t.TempDir()
	code, _, stderr := invoke(t, "-C", root, "init", "--yes")
	if code != exitOK {
		t.Fatalf("init: exit %d\n%s", code, stderr)
	}
	if got, want := layersIn(t, root), []string{"ilk/compound-lessons", "ilk/secrets"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("config lists %v, want %v", got, want)
	}
	if !strings.Contains(stderr, "skipping ilk/gates") || !strings.Contains(stderr, "test.command") {
		t.Errorf("init should say why gates was left out:\n%s", stderr)
	}
}

func TestInitMinimalIsTheCoreTierAlone(t *testing.T) {
	root := t.TempDir()
	if code, _, stderr := invoke(t, "-C", root, "init", "--yes", "--profile", "minimal"); code != exitOK {
		t.Fatalf("init: exit %d\n%s", code, stderr)
	}
	if got := layersIn(t, root); len(got) != 0 {
		t.Fatalf("minimal lists %v; the core tier is implied, not listed", got)
	}
	code, stdout, _ := invoke(t, "-C", root, "--json", "list")
	if code != exitOK {
		t.Fatalf("list: exit %d", code)
	}
	var rows []layerJSON
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.ID)
		if !r.Core {
			t.Errorf("%s is listed without core: true", r.ID)
		}
	}
	if got := strings.Join(ids, ","); got != "ilk/record,ilk/toolkit" {
		t.Fatalf("list shows %s, want the core tier", got)
	}
}

func TestACoreLayerCannotBeRemovedOrAdded(t *testing.T) {
	root := emptyRepo(t)
	for _, tc := range []struct{ args []string }{
		{[]string{"rm", "record"}},
		{[]string{"rm", "ilk/toolkit"}},
		{[]string{"add", "record"}},
	} {
		args := append([]string{"-C", root, "--json"}, tc.args...)
		code, _, stderr := invoke(t, args...)
		if code != exitUsage {
			t.Errorf("%v: exit %d, want %d", tc.args, code, exitUsage)
			continue
		}
		c, fix := errorOf(t, stderr)
		if c != "core_layer" || !strings.Contains(fix, "ilk config set") {
			t.Errorf("%v: code %q fix %q", tc.args, c, fix)
		}
	}
	if got := layersIn(t, root); len(got) != 0 {
		t.Fatalf("a refused add must not touch the config: %v", got)
	}
}

func TestApplyBringsInTheCoreTierAConfigLeavesOut(t *testing.T) {
	root := emptyRepo(t)
	if code, _, stderr := invoke(t, "-C", root, "apply", "--yes"); code != exitOK {
		t.Fatalf("apply: exit %d\n%s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "plans", "README.md")); err != nil {
		t.Fatalf("apply should lay out the record though the config never lists it: %v", err)
	}
	if code, _, stderr := invoke(t, "-C", root, "upgrade", "--yes"); code != exitOK {
		t.Fatalf("upgrade: exit %d\n%s", code, stderr)
	}
	if got := layersIn(t, root); len(got) != 0 {
		t.Fatalf("upgrade wrote the implied core into the config: %v", got)
	}
}

func TestACoreLayerIsListedOnlyWhileItCarriesAChoice(t *testing.T) {
	root := emptyRepo(t)
	if code, _, stderr := invoke(t, "-C", root, "config", "set", "record.review_after_commits", "20", "--yes"); code != exitOK {
		t.Fatalf("config set: exit %d\n%s", code, stderr)
	}
	if got := layersIn(t, root); !reflect.DeepEqual(got, []string{"ilk/record"}) {
		t.Fatalf("a set variable needs an entry to live in, got %v", got)
	}
	if code, _, stderr := invoke(t, "-C", root, "config", "unset", "record.review_after_commits", "--yes"); code != exitOK {
		t.Fatalf("config unset: exit %d\n%s", code, stderr)
	}
	if got := layersIn(t, root); len(got) != 0 {
		t.Fatalf("back on its defaults, the core layer needs no entry, got %v", got)
	}
}

func TestSearchLeavesTheCoreOutAndInfoStillDescribesIt(t *testing.T) {
	root := emptyRepo(t)
	code, stdout, _ := invoke(t, "-C", root, "--json", "search", "--all")
	if code != exitOK {
		t.Fatalf("search: exit %d", code)
	}
	for _, id := range []string{"ilk/record", "ilk/toolkit"} {
		if strings.Contains(stdout, `"`+id+`"`) {
			t.Errorf("search --all lists the core layer %s", id)
		}
	}
	code, stdout, _ = invoke(t, "-C", root, "--json", "info", "record")
	if code != exitOK {
		t.Fatalf("info record: exit %d", code)
	}
	var info struct {
		ID   string `json:"id"`
		Core bool   `json:"core"`
	}
	if err := json.Unmarshal([]byte(stdout), &info); err != nil {
		t.Fatal(err)
	}
	if info.ID != "ilk/record" || !info.Core {
		t.Fatalf("info record: %+v", info)
	}
}
