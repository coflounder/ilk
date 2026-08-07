package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/coflounder/ilk/internal/render"
	"github.com/coflounder/ilk/internal/repo"
)

// A command check runs a script the layer shipped, and that script needs the
// layer's configuration. Interpolating it into the command string works until the
// first value containing a space or a quote, which is exactly the kind of failure
// that shows up in somebody else's repository and not in ours.

func TestACommandCheckSeesTheLayersVariables(t *testing.T) {
	root := t.TempDir()
	p, err := engine.NewProject(&repo.Repo{Root: root}, config.Default(), lock.New(), "test")
	if err != nil {
		t.Fatal(err)
	}

	r := registered{
		check: manifest.Check{ID: "env.probe", Run: `printf '%s' "$ILK_VAR_OWNER" > owner; printf '%s' "$ILK_LAYER" > layer`},
		layer: "test/fake",
		ctx:   render.Context{Vars: map[string]string{"owner": "acme corp"}},
	}

	res := runOne(p, r, map[string]string{}, Options{})
	if res.Status != StatusPass {
		t.Fatalf("check did not pass: %s %s %s", res.Status, res.Reason, res.Output)
	}
	if got := readAt(t, root, "owner"); got != "acme corp" {
		t.Fatalf("the script saw owner %q, want %q", got, "acme corp")
	}
	if got := readAt(t, root, "layer"); got != "test/fake" {
		t.Fatalf("the script saw layer %q, want %q", got, "test/fake")
	}
}

func readAt(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}
