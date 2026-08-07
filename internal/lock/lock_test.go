package lock

import (
	"os"
	"path/filepath"
	"testing"
)

// A lockfile ilk cannot read is worse than no lockfile at all. With no lockfile
// ilk knows it is starting fresh; with an unreadable one it believes it has
// written nothing, stops recognising its own artifacts, and abandons every file
// it was supposed to be able to take back or detect edits to.
//
// The `layers` spelling predates the rename to `owners`. It has to keep loading.
func TestALockfileWrittenBeforeTheRenameStillLoads(t *testing.T) {
	root := t.TempDir()
	legacy := `{
  "version": 1,
  "layers": [
    {
      "id": "ilk/record",
      "version": "0.1.0",
      "source": "builtin",
      "files": [
        {"path": "plans/.gitkeep", "mode": "managed", "hash": "sha256:x", "owner": "ilk/record"}
      ],
      "baseline": ["docs/old.md"]
    },
    {"id": "ilk/core", "files": [{"path": ".gitignore", "mode": "append-once", "region": "core"}]},
    {"id": "target:claude-code", "files": [{"path": "CLAUDE.md", "mode": "region", "region": "pointer"}]}
  ],
  "targets": ["claude-code"]
}`
	writeLockfile(t, root, legacy)

	lk, err := Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(lk.Owners) != 3 {
		t.Fatalf("got %d owners, want 3 — an unreadable lockfile makes ilk forget what it wrote", len(lk.Owners))
	}

	rec, ok := lk.Owner("ilk/record")
	if !ok {
		t.Fatal("ilk/record was not carried over")
	}
	if len(rec.Files) != 1 || rec.Files[0].Path != "plans/.gitkeep" {
		t.Errorf("the recorded artifacts were lost: %+v", rec.Files)
	}
	if len(rec.Baseline) != 1 || rec.Baseline[0] != "docs/old.md" {
		t.Errorf("the baseline exemptions were lost: %+v", rec.Baseline)
	}

	// Kind is derived on read, so an old lockfile classifies correctly without a
	// migration step anybody has to run.
	for id, want := range map[string]Kind{
		"ilk/record":         KindLayer,
		"ilk/core":           KindCore,
		"target:claude-code": KindTarget,
	} {
		o, ok := lk.Owner(id)
		if !ok {
			t.Errorf("%s missing", id)
			continue
		}
		if o.Kind != want {
			t.Errorf("%s classified as %q, want %q", id, o.Kind, want)
		}
	}

	if got := lk.LayerIDs(); len(got) != 1 || got[0] != "ilk/record" {
		t.Errorf("LayerIDs should name only real layers, got %v", got)
	}
}

// Round-tripping must produce the current spelling, so the compatibility shim is
// needed exactly once per repository.
func TestSavingRewritesALegacyLockfileToTheCurrentSpelling(t *testing.T) {
	root := t.TempDir()
	writeLockfile(t, root, `{"version":1,"layers":[{"id":"ilk/record","files":[]}]}`)

	lk, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := lk.Save(root); err != nil {
		t.Fatal(err)
	}

	again, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Owners) != 1 {
		t.Fatalf("got %d owners after a save/load round trip, want 1", len(again.Owners))
	}
	data, err := os.ReadFile(Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if want := `"owners"`; !contains(string(data), want) {
		t.Errorf("the saved lockfile does not use %s:\n%s", want, data)
	}
}

func TestAMissingLockfileIsAValidStartingState(t *testing.T) {
	lk, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("a repository ilk has never applied to must load cleanly: %v", err)
	}
	if len(lk.Owners) != 0 {
		t.Errorf("expected no owners, got %v", lk.Owners)
	}
}

func writeLockfile(t *testing.T, root, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".ilk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(root), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	}()
}
