package layer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeManifest(t *testing.T, dir, version string) {
	t.Helper()
	m := "id: test/pinned\nversion: " + version + "\nsummary: A layer pinned to a commit.\n"
	if err := os.WriteFile(filepath.Join(dir, ManifestName), []byte(m), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A commit is the one pin that cannot move. `git clone --branch` refuses one,
// so a remote layer pinned to a commit has to be fetched by id instead.
func TestLoadGitPinnedToCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	src := t.TempDir()
	git(t, src, "init", "--quiet", "--initial-branch", "main")
	git(t, src, "config", "uploadpack.allowReachableSHA1InWant", "true")
	writeManifest(t, src, "0.1.0")
	git(t, src, "add", ".")
	git(t, src, "commit", "--quiet", "-m", "first")
	first := git(t, src, "rev-parse", "HEAD")
	writeManifest(t, src, "0.2.0")
	git(t, src, "commit", "--quiet", "-am", "second")

	cache := t.TempDir()
	pinned, err := loadGit(Ref{Raw: "pinned@" + first, Kind: KindGit, URL: "file://" + src, Version: first}, cache)
	if err != nil {
		t.Fatalf("loading a layer pinned to a commit: %v", err)
	}
	if pinned.Manifest.Version != "0.1.0" {
		t.Errorf("pinned to the first commit but loaded version %s", pinned.Manifest.Version)
	}
	if pinned.Digest != "git:"+first {
		t.Errorf("digest %s, want git:%s", pinned.Digest, first)
	}

	head, err := loadGit(Ref{Raw: "pinned", Kind: KindGit, URL: "file://" + src}, cache)
	if err != nil {
		t.Fatalf("loading the default branch: %v", err)
	}
	if head.Manifest.Version != "0.2.0" {
		t.Errorf("unpinned load gave version %s, want the branch's 0.2.0", head.Manifest.Version)
	}
}

func TestLoadGitUnknownCommitLeavesNoCache(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	src := t.TempDir()
	git(t, src, "init", "--quiet", "--initial-branch", "main")
	writeManifest(t, src, "0.1.0")
	git(t, src, "add", ".")
	git(t, src, "commit", "--quiet", "-m", "first")

	cache := t.TempDir()
	missing := strings.Repeat("a", 40)
	if _, err := loadGit(Ref{Raw: "x@" + missing, Kind: KindGit, URL: "file://" + src, Version: missing}, cache); err == nil {
		t.Fatal("loading a commit that does not exist succeeded")
	}
	entries, _ := os.ReadDir(cache)
	if len(entries) != 0 {
		t.Errorf("a failed fetch left %d cache entries, so the next attempt would trust a half-made checkout", len(entries))
	}
}
