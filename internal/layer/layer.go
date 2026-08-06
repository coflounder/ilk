// Package layer resolves a layer reference to a loaded layer: its manifest plus
// the file tree its templates live in.
//
// Sources are, in order of resolution: a layer built into the binary, a local
// path, and a git repository. Built-in layers are what make `ilk init` work with
// no network access at all.
package layer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/builtin"
	"github.com/coflounder/ilk/internal/manifest"
)

// ManifestName is the file every layer must contain.
const ManifestName = "layer.yaml"

// Loaded is a layer resolved from some source and ready to plan against.
type Loaded struct {
	Manifest *manifest.Layer
	// FS is rooted at the layer directory, so manifest `src:` paths resolve
	// against it directly.
	FS fs.FS
	// Source is the canonical, re-resolvable reference.
	Source string
	// Digest is a content hash of the layer tree, recorded in the lockfile so
	// drift in a layer's own source is detectable.
	Digest string
}

// Ref is a parsed layer reference.
type Ref struct {
	Raw     string
	Kind    Kind
	Name    string // builtin name or repo path
	Version string // requested version or git ref
	Subdir  string
	URL     string
}

// Kind enumerates where a layer comes from.
type Kind string

const (
	KindBuiltin Kind = "builtin"
	KindLocal   Kind = "local"
	KindGit     Kind = "git"
)

// ParseRef interprets a user-supplied layer reference.
func ParseRef(raw string) (Ref, error) {
	r := Ref{Raw: raw}
	if strings.TrimSpace(raw) == "" {
		return r, errors.New("empty layer reference")
	}

	spec := raw
	// Split a trailing @version, taking care not to eat the user in an scp-style
	// git URL (git@github.com:owner/repo).
	if at := strings.LastIndex(spec, "@"); at > 0 && !strings.Contains(spec[at:], ":") {
		r.Version = spec[at+1:]
		spec = spec[:at]
	}

	switch {
	case strings.HasPrefix(spec, "gh:"):
		r.Kind = KindGit
		rest := strings.TrimPrefix(spec, "gh:")
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) < 2 {
			return r, fmt.Errorf("%q is not a valid GitHub reference — use gh:owner/repo[/subdir][@version]", raw)
		}
		r.URL = "https://github.com/" + parts[0] + "/" + parts[1]
		r.Name = parts[0] + "/" + parts[1]
		if len(parts) == 3 {
			r.Subdir = parts[2]
		}
	case strings.HasPrefix(spec, "https://"), strings.HasPrefix(spec, "http://"),
		strings.HasPrefix(spec, "git@"), strings.HasPrefix(spec, "ssh://"):
		r.Kind = KindGit
		r.URL = spec
		r.Name = spec
	case strings.HasPrefix(spec, "."), strings.HasPrefix(spec, "/"), strings.HasPrefix(spec, "~"):
		r.Kind = KindLocal
		r.Name = spec
	default:
		// A bare name, with or without a namespace. Built-in first; the caller
		// falls back to a local directory of the same name if that misses.
		r.Kind = KindBuiltin
		r.Name = spec
	}
	return r, nil
}

// Resolve loads a layer from a reference. cacheDir is where git sources are
// materialised (normally .ilk/cache).
func Resolve(raw, cacheDir string) (*Loaded, error) {
	ref, err := ParseRef(raw)
	if err != nil {
		return nil, err
	}
	switch ref.Kind {
	case KindBuiltin:
		if l, err := loadBuiltin(ref.Name); err == nil {
			return l, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		// A bare name that is not built in may still be a directory.
		if info, statErr := os.Stat(ref.Name); statErr == nil && info.IsDir() {
			return loadLocal(ref.Name)
		}
		return nil, fmt.Errorf("unknown layer %q — `ilk list --available` shows the built-in layers, or use a path or gh:owner/repo", raw)
	case KindLocal:
		return loadLocal(expandHome(ref.Name))
	case KindGit:
		return loadGit(ref, cacheDir)
	}
	return nil, fmt.Errorf("unsupported layer reference %q", raw)
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func loadBuiltin(name string) (*Loaded, error) {
	// Accept both `record` and `ilk/record`.
	short := name
	if _, after, ok := strings.Cut(name, "/"); ok {
		short = after
	}
	sub, err := fs.Sub(builtin.FS, path.Join("layers", short))
	if err != nil {
		return nil, fs.ErrNotExist
	}
	data, err := fs.ReadFile(sub, ManifestName)
	if err != nil {
		return nil, fs.ErrNotExist
	}
	m, err := manifest.Parse(data, "builtin:"+short)
	if err != nil {
		return nil, err
	}
	if name != short && m.ID != name {
		return nil, fmt.Errorf("layer %q resolves to built-in %q — drop the namespace or use the full id", name, m.ID)
	}
	digest, err := digestFS(sub)
	if err != nil {
		return nil, err
	}
	return &Loaded{Manifest: m, FS: sub, Source: "builtin", Digest: digest}, nil
}

func loadLocal(dir string) (*Loaded, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(abs, ManifestName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%s has no %s — a layer directory must contain one", dir, ManifestName)
	}
	if err != nil {
		return nil, err
	}
	m, err := manifest.Parse(data, dir)
	if err != nil {
		return nil, err
	}
	fsys := os.DirFS(abs)
	digest, err := digestFS(fsys)
	if err != nil {
		return nil, err
	}
	return &Loaded{Manifest: m, FS: fsys, Source: abs, Digest: digest}, nil
}

func loadGit(ref Ref, cacheDir string) (*Loaded, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, errors.New("git is required to fetch remote layers but was not found on PATH")
	}
	key := sha256.Sum256([]byte(ref.URL + "@" + ref.Version))
	dest := filepath.Join(cacheDir, hex.EncodeToString(key[:8]))

	if _, err := os.Stat(filepath.Join(dest, ".git")); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, err
		}
		args := []string{"clone", "--quiet", "--depth", "1"}
		if ref.Version != "" {
			args = append(args, "--branch", ref.Version)
		}
		args = append(args, ref.URL, dest)
		cmd := exec.Command("git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("fetching %s: %w\n%s", ref.URL, err, strings.TrimSpace(string(out)))
		}
	}

	dir := dest
	if ref.Subdir != "" {
		dir = filepath.Join(dest, filepath.FromSlash(ref.Subdir))
	}
	loaded, err := loadLocal(dir)
	if err != nil {
		return nil, err
	}
	loaded.Source = ref.Raw
	if sha, err := gitHeadSHA(dest); err == nil {
		loaded.Digest = "git:" + sha
	}
	return loaded, nil
}

func gitHeadSHA(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// digestFS hashes a layer tree so the lockfile can detect a layer's own source
// changing underneath a repository.
func digestFS(fsys fs.FS) (string, error) {
	var paths []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		paths = append(paths, p)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%x\x00", p, sha256.Sum256(data))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// Builtins lists the layers compiled into the binary, in a stable order.
func Builtins() ([]*Loaded, error) {
	entries, err := fs.ReadDir(builtin.FS, "layers")
	if err != nil {
		return nil, err
	}
	var out []*Loaded
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		l, err := loadBuiltin(e.Name())
		if err != nil {
			return nil, fmt.Errorf("built-in layer %s: %w", e.Name(), err)
		}
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.ID < out[j].Manifest.ID })
	return out, nil
}

// ReadSource reads a template file from the layer tree.
func (l *Loaded) ReadSource(rel string) (string, error) {
	data, err := fs.ReadFile(l.FS, path.Clean(rel))
	if err != nil {
		return "", fmt.Errorf("layer %s: cannot read %s: %w", l.Manifest.ID, rel, err)
	}
	return string(data), nil
}

// ResolveVars merges declared defaults with the values a repository chose,
// rejecting any value outside a declared enum.
func (l *Loaded) ResolveVars(chosen map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(l.Manifest.Variables))
	for name, v := range l.Manifest.Variables {
		out[name] = v.Default
	}
	for name, value := range chosen {
		if _, ok := l.Manifest.Variables[name]; !ok {
			return nil, fmt.Errorf("layer %s has no variable %q", l.Manifest.ID, name)
		}
		out[name] = value
	}
	for name, v := range l.Manifest.Variables {
		if len(v.Enum) == 0 {
			continue
		}
		if !slicesContains(v.Enum, out[name]) {
			return nil, fmt.Errorf("layer %s: %s must be one of %s (got %q)", l.Manifest.ID, name, strings.Join(v.Enum, ", "), out[name])
		}
	}
	return out, nil
}

func slicesContains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
