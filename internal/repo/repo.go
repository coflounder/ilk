// Package repo locates the repository ilk is operating on and answers the few
// questions ilk asks about it. ilk is project-scoped and monorepo-only: there is
// exactly one root, and it is the git root.
package repo

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// DirName is the single directory holding all ilk state and configuration.
const DirName = ".ilk"

// Repo is a resolved repository root.
type Repo struct {
	Root string
}

// ErrNotFound is returned when no repository root can be located.
var ErrNotFound = errors.New("not inside a git repository or an ilk project: run `git init` first, then `ilk init`")

// Find walks up from start looking for a .ilk directory, then for a .git
// directory. Preferring .ilk lets ilk work in a directory that is not yet a git
// repository, while still snapping to the git root in the normal case.
func Find(start string) (*Repo, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	if root, ok := walkUp(abs, DirName); ok {
		return &Repo{Root: root}, nil
	}
	if root, ok := walkUp(abs, ".git"); ok {
		return &Repo{Root: root}, nil
	}
	return nil, ErrNotFound
}

// FindOrInit resolves a root for `ilk init`, which may run before .ilk exists.
func FindOrInit(start string) (*Repo, error) {
	if r, err := Find(start); err == nil {
		return r, nil
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	return &Repo{Root: abs}, nil
}

func walkUp(dir, marker string) (string, bool) {
	for {
		if info, err := os.Stat(filepath.Join(dir, marker)); err == nil && info.IsDir() {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// Path joins a repo-relative path onto the root.
func (r *Repo) Path(rel ...string) string {
	return filepath.Join(append([]string{r.Root}, rel...)...)
}

// IlkPath joins a path inside the .ilk directory.
func (r *Repo) IlkPath(rel ...string) string {
	return filepath.Join(append([]string{r.Root, DirName}, rel...)...)
}

// Name is the repository's directory name, used to title generated documents.
func (r *Repo) Name() string {
	return filepath.Base(r.Root)
}

// IsGit reports whether the root is a git repository.
func (r *Repo) IsGit() bool {
	info, err := os.Stat(filepath.Join(r.Root, ".git"))
	return err == nil && (info.IsDir() || info.Mode().IsRegular())
}

// GitDir returns the resolved .git directory, following the `gitdir:` pointer
// used by worktrees and submodules.
func (r *Repo) GitDir() (string, error) {
	p := filepath.Join(r.Root, ".git")
	info, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return p, nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	target := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if target == "" {
		return "", errors.New(".git file has no gitdir pointer")
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(r.Root, target)
	}
	return target, nil
}

// ChangedFilesSince returns paths modified more recently than the given git
// revision. It is best-effort: callers fall back to mtimes when git is absent.
func (r *Repo) ChangedFilesSince(rev string) ([]string, bool) {
	if !r.IsGit() {
		return nil, false
	}
	cmd := exec.Command("git", "diff", "--name-only", rev)
	cmd.Dir = r.Root
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, line)
		}
	}
	return files, true
}

// Commit is the little git knows about a change that a staleness check needs.
type Commit struct {
	SHA     string
	When    int64
	Subject string
}

// CommitsTouching returns commits after `since` that changed any of the given
// pathspecs, most recent first.
//
// This is what lets a document's staleness be measured against the code it
// actually describes, rather than against the calendar. A subsystem nobody has
// touched cannot make its documentation stale, however old that documentation is.
func (r *Repo) CommitsTouching(patterns []string, since int64, limit int) ([]Commit, bool) {
	if !r.IsGit() || len(patterns) == 0 {
		return nil, false
	}

	args := []string{"log", "--no-merges", fmt.Sprintf("--since=@%d", since),
		"--format=%H%x1f%at%x1f%s"}
	if limit > 0 {
		args = append(args, fmt.Sprintf("-%d", limit))
	}
	args = append(args, "--")
	args = append(args, gitPathspecs(patterns)...)

	return r.runLog(args)
}

func (r *Repo) runLog(args []string) ([]Commit, bool) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Root
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	var commits []Commit
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 3)
		if len(parts) != 3 {
			continue
		}
		when, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		commits = append(commits, Commit{SHA: parts[0], When: when, Subject: parts[2]})
	}
	return commits, true
}

// LastCommitSHA returns the most recent commit touching path.
func (r *Repo) LastCommitSHA(path string) (string, bool) {
	if !r.IsGit() {
		return "", false
	}
	cmd := exec.Command("git", "log", "-1", "--format=%H", "--", path)
	cmd.Dir = r.Root
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	sha := strings.TrimSpace(string(out))
	return sha, sha != ""
}

// CommitsInRange returns commits reachable from HEAD but not from `since` that
// touched the given pathspecs, most recent first.
//
// A commit range is exact where a timestamp is not: several commits can share a
// second, and `--since` would then include the very commit being measured from.
func (r *Repo) CommitsInRange(since string, patterns []string, limit int) ([]Commit, bool) {
	if !r.IsGit() || len(patterns) == 0 || since == "" {
		return nil, false
	}
	args := []string{"log", "--no-merges", since + "..HEAD", "--format=%H%x1f%at%x1f%s"}
	if limit > 0 {
		args = append(args, fmt.Sprintf("-%d", limit))
	}
	args = append(args, "--")
	args = append(args, gitPathspecs(patterns)...)
	return r.runLog(args)
}

// MatchesAnything reports whether a pathspec matches a tracked file. A pattern
// that matches nothing is worse than no pattern at all: it silently exempts the
// document from ever being checked.
func (r *Repo) MatchesAnything(pattern string) bool {
	if !r.IsGit() {
		return true // Nothing to verify against; do not invent a failure.
	}
	cmd := exec.Command("git", append([]string{"ls-files", "--"}, gitPathspecs([]string{pattern})...)...)
	cmd.Dir = r.Root
	out, err := cmd.Output()
	if err != nil {
		return true
	}
	return strings.TrimSpace(string(out)) != ""
}

// gitPathspecs marks glob patterns so git treats `**` the way the rest of the
// world does, rather than as a literal.
func gitPathspecs(patterns []string) []string {
	out := make([]string, 0, len(patterns))
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.ContainsAny(p, "*?[") {
			p = ":(glob)" + p
		}
		out = append(out, p)
	}
	return out
}

// LastCommitTime returns the author time of the most recent commit touching
// path, as a unix timestamp. ok is false when git has no record of the path.
func (r *Repo) LastCommitTime(path string) (int64, bool) {
	if !r.IsGit() {
		return 0, false
	}
	cmd := exec.Command("git", "log", "-1", "--format=%at", "--", path)
	cmd.Dir = r.Root
	out, err := cmd.Output()
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, false
	}
	var ts int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		ts = ts*10 + int64(c-'0')
	}
	return ts, true
}
