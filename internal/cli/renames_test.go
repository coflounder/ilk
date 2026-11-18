package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/builtin"
	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/layer"
)

// These tests hold the names the CLI settled on in the rename: --dry-run,
// --on-conflict, agents rm, contribute draft|submit|status, self build, search
// in place of list --available, and agents: in the config. The old spellings
// are gone outright, so each test also checks that the old one is a usage error
// rather than something that quietly does nothing.

// treeFiles reads every file under root, so a test can tell whether anything at
// all was written.
func treeFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		files[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func assertUnchanged(t *testing.T, what string, before, after map[string]string) {
	t.Helper()
	if reflect.DeepEqual(before, after) {
		return
	}
	for path, body := range after {
		if old, ok := before[path]; !ok {
			t.Errorf("%s wrote %s", what, path)
		} else if old != body {
			t.Errorf("%s changed %s", what, path)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			t.Errorf("%s removed %s", what, path)
		}
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	t.Run("apply", func(t *testing.T) {
		root := contractRepo(t)
		before := treeFiles(t, root)
		for _, args := range [][]string{
			{"-C", root, "apply", "--dry-run"},
			{"-C", root, "apply", "--dry-run", "--yes"},
			{"-C", root, "--json", "apply", "--dry-run"},
		} {
			code, stdout, stderr := invoke(t, args...)
			if code != exitOK {
				t.Fatalf("%v: exit %d\n%s", args, code, stderr)
			}
			if !strings.Contains(stdout, "owned.md") {
				t.Errorf("%v should still show the plan, got:\n%s", args, stdout)
			}
		}
		assertUnchanged(t, "apply --dry-run", before, treeFiles(t, root))
	})

	t.Run("add", func(t *testing.T) {
		root := emptyRepo(t)
		mem := memoryLayer(t, root, "claude-mem")
		before := treeFiles(t, root)
		for _, args := range [][]string{
			{"-C", root, "add", mem, "--dry-run"},
			{"-C", root, "--json", "add", mem, "--dry-run"},
		} {
			if code, _, stderr := invoke(t, args...); code != exitOK {
				t.Fatalf("%v: exit %d\n%s", args, code, stderr)
			}
		}
		assertUnchanged(t, "add --dry-run", before, treeFiles(t, root))
		if got := layersIn(t, root); len(got) != 0 {
			t.Errorf("add --dry-run recorded %v in the config", got)
		}
	})

	t.Run("rm", func(t *testing.T) {
		root := contractRepo(t)
		if code, _, stderr := invoke(t, "-C", root, "apply", "--yes"); code != exitOK {
			t.Fatalf("apply: exit %d\n%s", code, stderr)
		}
		before := treeFiles(t, root)
		for _, args := range [][]string{
			{"-C", root, "rm", "test/echoer", "--dry-run"},
			{"-C", root, "--json", "rm", "test/echoer", "--dry-run"},
		} {
			if code, _, stderr := invoke(t, args...); code != exitOK {
				t.Fatalf("%v: exit %d\n%s", args, code, stderr)
			}
		}
		assertUnchanged(t, "rm --dry-run", before, treeFiles(t, root))
	})

	t.Run("upgrade", func(t *testing.T) {
		// Never applied, so upgrading has the whole layer to write.
		root := contractRepo(t)
		before := treeFiles(t, root)
		for _, args := range [][]string{
			{"-C", root, "upgrade", "--dry-run"},
			{"-C", root, "--json", "upgrade", "--dry-run"},
		} {
			code, stdout, stderr := invoke(t, args...)
			if code != exitOK {
				t.Fatalf("%v: exit %d\n%s", args, code, stderr)
			}
			if !strings.Contains(stdout, "owned.md") {
				t.Errorf("%v should still show the plan, got:\n%s", args, stdout)
			}
		}
		assertUnchanged(t, "upgrade --dry-run", before, treeFiles(t, root))
	})

	t.Run("init", func(t *testing.T) {
		root := t.TempDir()
		if code, _, stderr := invoke(t, "-C", root, "init", "--dry-run"); code != exitOK {
			t.Fatalf("init --dry-run: exit %d\n%s", code, stderr)
		}
		if got := treeFiles(t, root); len(got) != 0 {
			t.Errorf("init --dry-run wrote %d file(s), want none", len(got))
		}
	})

	t.Run("the old flag is gone", func(t *testing.T) {
		root := contractRepo(t)
		for _, args := range [][]string{
			{"-C", root, "add", "x", "--no-apply"},
			{"-C", root, "init", "--no-apply"},
		} {
			if code, _, _ := invoke(t, args...); code != exitUsage {
				t.Errorf("%v: exit %d, want %d", args, code, exitUsage)
			}
		}
	})
}

func TestOnConflictValuesMapToPlanOptions(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want engine.PlanOptions
	}{
		{"merge", engine.PlanOptions{}},
		{"markers", engine.PlanOptions{MergeMarkers: true}},
		{"keep-mine", engine.PlanOptions{Accept: true}},
		{"take-layer", engine.PlanOptions{Force: true}},
		{"refuse", engine.PlanOptions{NoMerge: true}},
	} {
		got, err := conflictOptions(tc.mode)
		if err != nil {
			t.Errorf("%s: %v", tc.mode, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.mode, got, tc.want)
		}
	}
	if len(conflictModes) != 5 {
		t.Errorf("conflictModes lists %d values; keep it in step with conflictOptions", len(conflictModes))
	}
}

func TestOnConflictDefaultIsMerge(t *testing.T) {
	for _, path := range [][]string{{"add"}, {"rm"}, {"upgrade"}, {"plan"}, {"apply"}} {
		root := newRoot()
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Fatal(err)
		}
		f := cmd.Flags().Lookup("on-conflict")
		if f == nil {
			t.Errorf("ilk %s has no --on-conflict", path[0])
			continue
		}
		if f.DefValue != conflictMerge {
			t.Errorf("ilk %s --on-conflict defaults to %q, want %q", path[0], f.DefValue, conflictMerge)
		}
		for _, old := range []string{"force", "accept", "merge-markers", "no-merge", "no-apply"} {
			if cmd.Flags().Lookup(old) != nil {
				t.Errorf("ilk %s still has --%s", path[0], old)
			}
		}
	}
}

func TestAnInvalidOnConflictIsAUsageError(t *testing.T) {
	root := contractRepo(t)
	for _, cmd := range [][]string{{"apply"}, {"plan"}, {"upgrade"}, {"rm", "test/echoer"}, {"add", "anything"}} {
		args := append([]string{"-C", root, "--json"}, cmd...)
		args = append(args, "--on-conflict", "sideways")
		code, _, stderr := invoke(t, args...)
		if code != exitUsage {
			t.Errorf("%v: exit %d, want %d\n%s", cmd, code, exitUsage, stderr)
			continue
		}
		if got := jsonErrorCode(t, stderr); got != "invalid_on_conflict" {
			t.Errorf("%v: code %q, want invalid_on_conflict", cmd, got)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "owned.md")); err == nil {
		t.Error("an invalid --on-conflict still applied the plan")
	}
}

// Each value does what it says on a real edited file.
func TestOnConflictResolvesAnEditedFile(t *testing.T) {
	root := contractRepo(t)
	if code, _, stderr := invoke(t, "-C", root, "apply", "--yes"); code != exitOK {
		t.Fatalf("apply: exit %d\n%s", code, stderr)
	}
	owned := filepath.Join(root, "owned.md")
	mustWrite(t, owned, "mine\n")
	read := func() string {
		data, err := os.ReadFile(owned)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	// The layer has not moved, so there is nothing to merge: merge and refuse
	// both leave the edit alone and say so.
	for _, mode := range []string{"merge", "refuse"} {
		if code, _, _ := invoke(t, "-C", root, "apply", "--yes", "--on-conflict", mode); code != exitFailed {
			t.Errorf("%s: exit %d, want %d for a refused file", mode, code, exitFailed)
		}
		if read() != "mine\n" {
			t.Fatalf("%s overwrote the edit", mode)
		}
	}

	if code, _, stderr := invoke(t, "-C", root, "apply", "--yes", "--on-conflict", "keep-mine"); code != exitOK {
		t.Fatalf("keep-mine: exit %d\n%s", code, stderr)
	}
	if read() != "mine\n" {
		t.Fatal("keep-mine changed the file")
	}
	if code, _, _ := invoke(t, "-C", root, "plan", "--exit-code"); code != exitOK {
		t.Errorf("after keep-mine the file should be settled; plan --exit-code exits %d", code)
	}

	mustWrite(t, owned, "mine again\n")
	if code, _, stderr := invoke(t, "-C", root, "apply", "--yes", "--on-conflict", "take-layer"); code != exitOK {
		t.Fatalf("take-layer: exit %d\n%s", code, stderr)
	}
	if read() != "owned\n" {
		t.Errorf("take-layer left %q, want the layer's version", read())
	}
}

func TestAgentsRm(t *testing.T) {
	root := contractRepo(t)
	if code, _, stderr := invoke(t, "-C", root, "agents", "add", "codex", "--yes"); code != exitOK {
		t.Fatalf("agents add: exit %d\n%s", code, stderr)
	}
	if !loadConfig(t, root).HasAgent("codex") {
		t.Fatal("setup: codex was not added")
	}
	if code, _, stderr := invoke(t, "-C", root, "agents", "rm", "codex", "--yes"); code != exitOK {
		t.Fatalf("agents rm: exit %d\n%s", code, stderr)
	}
	if loadConfig(t, root).HasAgent("codex") {
		t.Error("agents rm left codex in the config")
	}

	// The old name is not a command any more, and must not look like one by
	// printing help and exiting 0.
	code, _, stderr := invoke(t, "-C", root, "--json", "agents", "remove", "codex")
	if code != exitUsage {
		t.Errorf("agents remove: exit %d, want %d\n%s", code, exitUsage, stderr)
	}
	if got := jsonErrorCode(t, stderr); got != "usage" {
		t.Errorf("agents remove: code %q, want usage", got)
	}
}

func TestContributeHasDraftSubmitAndStatus(t *testing.T) {
	root := newRoot()
	for _, name := range []string{"draft", "submit", "status"} {
		cmd, _, err := root.Find([]string{"contribute", name})
		if err != nil || cmd.Name() != name {
			t.Errorf("ilk contribute %s is not a command (found %q, %v)", name, cmd.Name(), err)
		}
	}
	if cmd, _, _ := root.Find([]string{"contribute", "submit"}); cmd.Flags().Lookup("yes") == nil {
		t.Error("ilk contribute submit has no --yes")
	}
	if cmd, _, _ := root.Find([]string{"contribute"}); cmd.Flags().Lookup("submit") != nil {
		t.Error("ilk contribute still has --submit")
	}

	repo := contractRepo(t)
	if code, _, stderr := invoke(t, "-C", repo, "contribute", "status"); code != exitOK {
		t.Errorf("contribute status: exit %d\n%s", code, stderr)
	}
	if code, _, stderr := invoke(t, "-C", repo, "--json", "contribute", "draft", "test/echoer"); code != exitOK {
		t.Errorf("contribute draft --json: exit %d\n%s", code, stderr)
	}
	// A layer name where a subcommand belongs is the old spelling.
	if code, _, _ := invoke(t, "-C", repo, "contribute", "test/echoer"); code != exitUsage {
		t.Errorf("contribute <layer>: exit %d, want %d", code, exitUsage)
	}
}

func TestSelfBuildTakesPath(t *testing.T) {
	root := newRoot()
	cmd, _, err := root.Find([]string{"self", "build"})
	if err != nil || cmd.Name() != "build" {
		t.Fatalf("ilk self build is not a command (found %q, %v)", cmd.Name(), err)
	}
	for _, flag := range []string{"path", "dest", "no-verify-source"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("ilk self build has no --%s", flag)
		}
	}
	if code, _, _ := invoke(t, "self", "update", "--path", "."); code != exitUsage {
		t.Errorf("self update --path: exit %d, want %d — it is self build now", code, exitUsage)
	}
	// A path that is not an ilk checkout is refused before the toolchain runs.
	code, _, stderr := invoke(t, "self", "build", "--path", t.TempDir(), "--dest", filepath.Join(t.TempDir(), "ilk"))
	if code != exitFailed || !strings.Contains(stderr, "not an ilk checkout") {
		t.Errorf("self build on a non-checkout: exit %d\n%s", code, stderr)
	}
}

// Search is the one way to find layers to add. Everything list --available used
// to show, every built-in not yet adopted, has to be in it.
func TestSearchShowsWhatListAvailableDid(t *testing.T) {
	root := emptyRepo(t)
	code, stdout, stderr := invoke(t, "-C", root, "--json", "search")
	if code != exitOK {
		t.Fatalf("search: exit %d\n%s", code, stderr)
	}
	var rows []struct {
		ID      string `json:"id"`
		Adopted bool   `json:"adopted"`
	}
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("search --json: %v\n%s", err, stdout)
	}
	listed := map[string]bool{}
	for _, r := range rows {
		listed[r.ID] = true
	}
	builtins, err := layer.Builtins()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range builtins {
		core := builtin.IsCore(b.Manifest.ID)
		if listed[b.Manifest.ID] == core {
			t.Errorf("search lists %s: %v, but it is core: %v — search lists every built-in but the core tier", b.Manifest.ID, listed[b.Manifest.ID], core)
		}
	}

	if code, _, _ := invoke(t, "-C", root, "list", "--available"); code != exitUsage {
		t.Errorf("list --available: exit %d, want %d — search replaces it", code, exitUsage)
	}
	if code, stdout, _ := invoke(t, "-C", root, "list"); code != exitOK || !strings.Contains(stdout, "ilk search") {
		t.Errorf("an empty list should point at ilk search: exit %d\n%s", code, stdout)
	}
}

func TestConfigNamesAgentsNotTargets(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, config.Path(root), "version: 1\nagents:\n  - claude-code\n  - codex\nlayers: []\n")
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("a config with agents: does not load: %v", err)
	}
	if !reflect.DeepEqual(cfg.Agents, []string{"claude-code", "codex"}) {
		t.Errorf("agents = %v", cfg.Agents)
	}

	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(config.Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\nagents:\n") || strings.Contains(string(data), "targets:") {
		t.Errorf("saved config should use agents:, got:\n%s", data)
	}

	// No shim: the old key is refused, not silently ignored.
	mustWrite(t, config.Path(root), "version: 1\ntargets:\n  - claude-code\nlayers: []\n")
	if _, err := config.Load(root); err == nil {
		t.Error("a config with targets: loaded; it should be refused")
	}
}
