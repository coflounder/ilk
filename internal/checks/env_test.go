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

// Which variable carries a credential is the backend's choice, not the layer's,
// so a requires_env entry may read a layer variable.
func TestRequiresEnvIsRenderedFromTheLayersVariables(t *testing.T) {
	root := t.TempDir()
	p, err := engine.NewProject(&repo.Repo{Root: root}, config.Default(), lock.New(), "test")
	if err != nil {
		t.Fatal(err)
	}
	probe := func(credentials string) Result {
		r := registered{
			check: manifest.Check{ID: "env.cred", Run: "true", RequiresEnv: []string{"{{ .Vars.credentials }}"}},
			layer: "test/fake",
			ctx:   render.Context{Vars: map[string]string{"credentials": credentials}},
		}
		return runOne(p, r, map[string]string{}, Options{})
	}

	t.Setenv("ILK_TEST_PRESENT_TOKEN", "x")
	if res := probe("ILK_TEST_ABSENT_TOKEN"); res.Status != StatusSkip || !strings.Contains(res.Reason, "ILK_TEST_ABSENT_TOKEN") {
		t.Errorf("an absent rendered variable should skip, naming it: %s %s", res.Status, res.Reason)
	}
	if res := probe("ILK_TEST_PRESENT_TOKEN"); res.Status != StatusPass {
		t.Errorf("a present rendered variable should let the check run: %s %s", res.Status, res.Reason)
	}
	if res := probe(""); res.Status != StatusPass {
		t.Errorf("an entry that renders empty requires nothing: %s %s", res.Status, res.Reason)
	}
	if res := probe("not a name"); res.Status != StatusError {
		t.Errorf("a rendered value that is not a variable name is an error, not a skip: %s %s", res.Status, res.Reason)
	}
}
