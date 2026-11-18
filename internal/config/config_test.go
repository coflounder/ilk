package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A config an earlier ilk wrote names its agents under `targets:`. Loading it
// must say what to rename, not report a decoder's unknown field.
func TestLoadNamesRenamedTargetsKey(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".ilk"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := "version: 1\ntargets:\n  - claude-code\nlayers: []\n"
	if err := os.WriteFile(Path(root), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(root)
	if err == nil {
		t.Fatal("a config with targets: loaded")
	}
	if !strings.Contains(err.Error(), "`targets:` is now called `agents:`") {
		t.Errorf("error does not name the rename: %v", err)
	}
}
