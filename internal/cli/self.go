package cli

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// Module is the import path `ilk self update` expects to find at --path.
// Building some other project over the ilk binary would be a confusing way to
// lose an afternoon, so the source is identified before the toolchain is invoked.
const Module = "github.com/coflounder/ilk"

// newSelfCmd groups the commands that act on the ilk binary rather than on a
// repository.
//
// The namespace is the point. Every other command here changes a repository; the
// ones under `self` change the tool. Without the distinction being structural,
// `ilk update` and `ilk upgrade` differ by one letter and by which of two very
// different things they touch — an ambiguity rustup resolved the same way.
func newSelfCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "self",
		Short: "Act on the ilk binary itself, rather than on a repository",
	}
	cmd.AddCommand(newSelfUpdateCmd())
	return cmd
}

// newSelfUpdateCmd rebuilds ilk from a source checkout and replaces the running
// binary with the result.
//
// It exists because of a trap the project documents about itself: the built-in
// layers are embedded in the binary, so a stale ilk silently serves stale
// templates. "Rebuild the binary" and "update the layers this repository uses"
// are the same act for record, gates and toolkit, and nothing said so at
// the command line until now.
func newSelfUpdateCmd() *cobra.Command {
	var (
		path    string
		dest    string
		noCheck bool
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Rebuild ilk from a source checkout and replace the running binary",
		Long: `Rebuild ilk from source and install it over the binary you are running.

This changes the tool. ` + "`ilk upgrade`" + ` changes a repository: it re-resolves the
layers that repository has, and never touches the binary.

The two are related, because the built-in layers — record, quality-gates and
toolkit — are compiled into the binary. Updating ilk is therefore how a repository
gets new built-in layer content, and ` + "`ilk upgrade`" + ` is how it takes delivery:

    ilk self update --path ../ilk    # rebuild this binary from that checkout
    ilk upgrade                      # reconcile this repository to what it embeds

The working tree is built as it stands, uncommitted changes included. That is the
point: this is the loop for developing ilk, not a way to fetch a release.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			src, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if err := verifyIlkSource(src, noCheck); err != nil {
				return err
			}

			goBin, err := exec.LookPath("go")
			if err != nil {
				return fmt.Errorf("`ilk self update` builds from source and needs the Go toolchain, which is not on PATH\n" +
					"  fix: install Go, or use a released binary instead")
			}

			target, err := updateTarget(dest)
			if err != nil {
				return err
			}

			before := binaryVersion(target)
			beforeSum := fileSum(target)

			// Build beside the destination rather than in a temp directory: the
			// swap below is a rename, and a rename is only atomic within one
			// filesystem.
			staged, err := os.CreateTemp(filepath.Dir(target), ".ilk-update-*")
			if err != nil {
				return fmt.Errorf("cannot write next to %s: %w", target, err)
			}
			stagedPath := staged.Name()
			staged.Close()
			defer os.Remove(stagedPath)

			build := exec.Command(goBin, "build", "-o", stagedPath, "./cmd/ilk")
			build.Dir = src
			build.Stdout = errOut
			build.Stderr = errOut
			if err := build.Run(); err != nil {
				return fmt.Errorf("building %s failed: %w", src, err)
			}
			if err := os.Chmod(stagedPath, 0o755); err != nil {
				return err
			}

			// Run what was just built before standing on it. A binary that cannot
			// report its own version is not one to replace a working ilk with.
			after := binaryVersion(stagedPath)
			if after == "" {
				return fmt.Errorf("the binary built from %s does not run — leaving %s alone", src, target)
			}

			// Rename rather than write: the destination is this process's own
			// executable, and an open executable cannot be written to. Replacing
			// the directory entry is fine, and is atomic.
			if err := os.Rename(stagedPath, target); err != nil {
				return fmt.Errorf("could not replace %s: %w\n"+
					"  fix: check you own the file, or pass --dest to install somewhere you can write", target, err)
			}

			afterSum := fileSum(target)
			rev := gitDescribe(src)

			// The version string is derived from the commit, so during development
			// it stays put while the code underneath moves. Reporting "unchanged"
			// on a rebuild that did change the binary would be a small lie told
			// exactly when somebody is checking whether their edit landed.
			changed := beforeSum != afterSum

			if flagJSON {
				return emitJSON(map[string]any{
					"source": src, "destination": target,
					"revision": rev, "before": before, "after": after,
					"changed": changed, "version_changed": before != after,
				})
			}

			printf("%s %s\n", sty.bold("update"), target)
			printf("  %s %s", sty.dim("from:"), sty.dim(src))
			if rev != "" {
				printf(" %s", sty.dim("("+rev+")"))
			}
			printf("\n")
			switch {
			case before != after:
				printf("\n%s %s → %s\n", sty.green("updated."), sty.dim(before), after)
			case changed:
				printf("\n%s %s %s\n", sty.green("updated."), after, sty.dim("(same version, new build)"))
			default:
				printf("\n%s %s %s\n", sty.green("rebuilt."), after, sty.dim("(identical binary)"))
			}
			if changed {
				printUpdateNextSteps()
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&path, "path", ".", "the ilk source checkout to build from")
	cmd.Flags().StringVar(&dest, "dest", "", "install to this path instead of the running binary")
	cmd.Flags().BoolVar(&noCheck, "no-verify-source", false, "skip the check that --path is an ilk checkout")
	return cmd
}

// verifyIlkSource rejects a path that is not an ilk checkout, before the Go
// toolchain gets a chance to build something else over the binary on PATH.
func verifyIlkSource(src string, skip bool) error {
	info, err := os.Stat(src)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s does not exist\n  fix: pass --path pointing at your ilk checkout, e.g. --path ../ilk", src)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory\n"+
			"  fix: --path takes the root of an ilk checkout, e.g. --path ../ilk", src)
	}
	if skip {
		return nil
	}

	data, err := os.ReadFile(filepath.Join(src, "go.mod"))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s has no go.mod, so it is not an ilk checkout\n"+
			"  fix: pass --path pointing at one, e.g. --path ../ilk", src)
	}
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "module "+Module {
			return nil
		}
	}
	return fmt.Errorf("%s is a Go module, but not %s\n"+
		"  Building it over your ilk binary would replace ilk with something else.\n"+
		"  fix: point --path at an ilk checkout, or pass --no-verify-source if you mean it", src, Module)
}

// updateTarget resolves what to overwrite: an explicit --dest, or the running
// binary. A symlinked entry resolves to the file behind it, so updating through
// a shim replaces the binary rather than the link.
func updateTarget(dest string) (string, error) {
	if dest != "" {
		abs, err := filepath.Abs(dest)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot work out which binary is running: %w\n  fix: pass --dest with where to install", err)
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	return self, nil
}

// fileSum hashes a file's contents, so a rebuild that produced different code
// can be distinguished from one that produced the same bytes. An unreadable
// file hashes to nothing, which reads as "changed" — the safe direction.
func fileSum(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// binaryVersion asks a binary what it is. An empty result means it did not run.
func binaryVersion(path string) string {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitDescribe reports the revision built, marking a dirty tree, because building
// uncommitted work is the normal case here and the output should say so.
func gitDescribe(dir string) string {
	rev, err := exec.Command("git", "-C", dir, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	out := strings.TrimSpace(string(rev))
	if status, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output(); err == nil {
		if strings.TrimSpace(string(status)) != "" {
			out += ", uncommitted changes"
		}
	}
	return out
}

// printUpdateNextSteps names the step that is easy to forget: a new binary means
// new built-in layers, and no repository has taken delivery of them yet.
func printUpdateNextSteps() {
	printf("\n%s\n", sty.dim("The built-in layers ship inside this binary. Repositories using them"))
	printf("%s\n", sty.dim("are a version behind until they reconcile:"))
	printf("  %-28s %s\n", "ilk upgrade", sty.dim("in each repository, to take delivery"))
}
