package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `ilk self update` builds a directory and installs the result over the binary on
// PATH. Getting the source wrong therefore replaces ilk with something else,
// which is both easy to do — `--path ..` in the wrong shell — and confusing
// afterwards, because the failure shows up as ilk behaving like another program.
func TestUpdateRefusesASourceThatIsNotIlk(t *testing.T) {
	root := t.TempDir()

	notAModule := filepath.Join(root, "plain")
	mustMkdir(t, notAModule)

	otherModule := filepath.Join(root, "other")
	mustMkdir(t, otherModule)
	mustWrite(t, filepath.Join(otherModule, "go.mod"), "module github.com/someone/else\n\ngo 1.24\n")

	aFile := filepath.Join(root, "file.txt")
	mustWrite(t, aFile, "not a directory\n")

	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{"absent", filepath.Join(root, "nope"), "does not exist"},
		{"a file", aFile, "not a directory"},
		{"no go.mod", notAModule, "not an ilk checkout"},
		{"another module", otherModule, "but not " + Module},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyIlkSource(tc.path, false)
			if err == nil {
				t.Fatalf("%s was accepted as an ilk checkout", tc.path)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error should say %q, got: %v", tc.want, err)
			}
			// Every refusal has to name the way forward, the same as a check does.
			if !strings.Contains(err.Error(), "fix:") {
				t.Errorf("the refusal does not say what to do: %v", err)
			}
		})
	}
}

func TestUpdateAcceptsAnIlkCheckout(t *testing.T) {
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "go.mod"), "module "+Module+"\n\ngo 1.24\n\nrequire (\n\tgithub.com/spf13/cobra v1.8.1\n)\n")
	if err := verifyIlkSource(src, false); err != nil {
		t.Fatalf("a real ilk checkout was refused: %v", err)
	}
}

// The escape hatch has to work, or somebody building a fork under a different
// module path has no way through except editing ilk.
func TestUpdateCanBeToldToSkipTheSourceCheck(t *testing.T) {
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "go.mod"), "module github.com/someone/fork\n\ngo 1.24\n")
	if err := verifyIlkSource(src, false); err == nil {
		t.Fatal("setup: the fork should be refused by default")
	}
	if err := verifyIlkSource(src, true); err != nil {
		t.Errorf("--no-verify-source should allow it: %v", err)
	}
}

func TestUpdateTargetPrefersAnExplicitDestination(t *testing.T) {
	got, err := updateTarget("some/where/ilk")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("the destination should be absolute, got %q", got)
	}
	if filepath.Base(got) != "ilk" {
		t.Errorf("the destination lost its filename: %q", got)
	}

	// With no --dest it resolves the running binary, which under `go test` is
	// the test binary. What matters is that it resolves to something real.
	self, err := updateTarget("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(self); err != nil {
		t.Errorf("resolved a destination that does not exist: %q", self)
	}
}

// fileSum is what distinguishes "rebuilt and nothing moved" from "rebuilt and
// your edit landed". During development the version string cannot tell them
// apart, because it comes from the commit.
func TestFileSumDistinguishesContent(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	mustWrite(t, a, "one")
	mustWrite(t, b, "two")

	if fileSum(a) == fileSum(b) {
		t.Error("different content hashed the same")
	}
	mustWrite(t, b, "one")
	if fileSum(a) != fileSum(b) {
		t.Error("identical content hashed differently")
	}
	if fileSum(filepath.Join(dir, "absent")) != "" {
		t.Error("a missing file should hash to nothing, so it reads as changed")
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
