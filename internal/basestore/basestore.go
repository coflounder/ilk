// Package basestore keeps a copy of what ilk last wrote to each artifact.
//
// The lockfile records a hash, which is enough to detect that a file changed but
// not enough to reconcile two changes to it. A three-way merge needs the common
// ancestor's text, so ilk stores it: content-addressed, under .ilk/base/, keyed
// by the same hash the lockfile already carries.
//
// The store is committed rather than ignored. A teammate who pulls the
// repository and runs `ilk upgrade` needs the same ancestor, or their upgrade
// degrades to a refusal for no reason they can see.
package basestore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DirName is the store's location inside .ilk/.
const DirName = "base"

// Dir returns the store's path for a repository root.
func Dir(root string) string { return filepath.Join(root, ".ilk", DirName) }

// pathFor maps a `sha256:abcdef…` hash to a file path. The two-character shard
// keeps directories small enough to list comfortably.
func pathFor(root, hash string) (string, error) {
	algo, digest, ok := strings.Cut(hash, ":")
	if !ok || algo == "" || len(digest) < 4 {
		return "", errors.New("malformed content hash: " + hash)
	}
	for _, r := range algo + digest {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
		isAlgo := r >= 'a' && r <= 'z'
		if !isHex && !isAlgo {
			return "", errors.New("malformed content hash: " + hash)
		}
	}
	return filepath.Join(Dir(root), algo, digest[:2], digest[2:]), nil
}

// Put stores content under its hash. Storing the same content twice is a no-op,
// which is what makes the store cheap for files that never change.
func Put(root, hash, content string) error {
	path, err := pathFor(root, hash)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// Get returns previously stored content. A miss is ordinary: the store may
// predate a repository, or have been pruned, and callers fall back to refusing
// rather than failing.
func Get(root, hash string) (string, bool) {
	path, err := pathFor(root, hash)
	if err != nil {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// GC removes stored content no longer referenced by the lockfile, so the store
// tracks the repository rather than accumulating every version it ever had.
func GC(root string, keep map[string]bool) (removed int, err error) {
	dir := Dir(root)
	if _, statErr := os.Stat(dir); errors.Is(statErr, os.ErrNotExist) {
		return 0, nil
	}

	kept := map[string]bool{}
	for hash := range keep {
		if path, pathErr := pathFor(root, hash); pathErr == nil {
			kept[path] = true
		}
	}

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || kept[path] {
			return nil
		}
		if rmErr := os.Remove(path); rmErr != nil {
			return rmErr
		}
		removed++
		return nil
	})
	if err != nil {
		return removed, err
	}

	pruneEmptyDirs(dir)
	return removed, nil
}

// pruneEmptyDirs clears out the shard directories a collection emptied.
func pruneEmptyDirs(root string) {
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})
	// Deepest first, so a parent emptied by its children also goes.
	for i := len(dirs) - 1; i >= 0; i-- {
		if entries, err := os.ReadDir(dirs[i]); err == nil && len(entries) == 0 {
			_ = os.Remove(dirs[i])
		}
	}
}
