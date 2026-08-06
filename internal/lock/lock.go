// Package lock reads and writes .ilk/lock.json — ilk's record of what it
// actually wrote.
//
// The lockfile is what makes drop and upgrade safe. Every file ilk touches is
// recorded with a hash of the content ilk put there, so ilk can tell the
// difference between "unchanged since I wrote it" (safe to overwrite or delete)
// and "a human has edited this" (stop and ask).
package lock

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/coflounder/ilk/internal/manifest"
)

// FileName is the lockfile's name inside .ilk/.
const FileName = "lock.json"

// Lock is .ilk/lock.json.
type Lock struct {
	Version int      `json:"version"`
	Layers  []Layer  `json:"layers"`
	Targets []string `json:"targets"`
}

// Layer is the recorded result of adopting one layer.
type Layer struct {
	ID      string            `json:"id"`
	Version string            `json:"version"`
	Source  string            `json:"source"`
	Digest  string            `json:"digest,omitempty"`
	Vars    map[string]string `json:"vars,omitempty"`
	Files   []File            `json:"files"`
	// Baseline lists files that were already in this layer's directories when it
	// was adopted. A layer governs what happens next, not what came before: a
	// repository that already had a `docs/` folder should not be greeted by a
	// wall of failures about files nobody has touched. These paths are exempt
	// from the layer's checks until somebody clears them with `ilk baseline`.
	Baseline []string `json:"baseline,omitempty"`
}

// File is one artifact ilk wrote.
type File struct {
	Path string        `json:"path"`
	Mode manifest.Mode `json:"mode"`
	// Region is set for region and append-once modes.
	Region string `json:"region,omitempty"`
	// Hash is what ilk expects to find: the whole file for managed mode, the
	// region body for region and append-once modes. It is empty for create-only,
	// which ilk deliberately stops tracking after the initial write. Anything
	// else means somebody has edited ilk's output.
	Hash string `json:"hash,omitempty"`
	// Delivered is the layer's own content at the last reconciliation, which is
	// the common ancestor a three-way merge needs.
	//
	// It differs from Hash whenever the file legitimately diverges from what the
	// layer produces — after a merge, or after `--accept`. Collapsing the two
	// would make the next apply mistake an agreed divergence for "unchanged since
	// ilk wrote it" and overwrite it.
	Delivered string `json:"delivered,omitempty"`
	// CreatedFile records that the file did not exist before ilk wrote it, which
	// is what allows drop to remove an emptied file rather than leaving a husk.
	CreatedFile bool `json:"created_file,omitempty"`
	// Owner names the producer, either a layer id or a target name, so `ilk
	// drop` and `ilk agents sync` can each clean up only their own output.
	Owner string `json:"owner,omitempty"`
	Exec  bool   `json:"exec,omitempty"`
}

// New returns an empty lock.
func New() *Lock {
	return &Lock{Version: 1, Layers: []Layer{}}
}

// Path returns the lockfile path for a repository root.
func Path(root string) string {
	return filepath.Join(root, ".ilk", FileName)
}

// Load reads the lockfile. A missing lockfile is not an error: it means ilk has
// never applied anything here, which is a valid starting state.
func Load(root string) (*Lock, error) {
	data, err := os.ReadFile(Path(root))
	if errors.Is(err, os.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return nil, err
	}
	var l Lock
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, err
	}
	if l.Layers == nil {
		l.Layers = []Layer{}
	}
	return &l, nil
}

// Save writes the lockfile deterministically, so it produces clean diffs.
func (l *Lock) Save(root string) error {
	sort.Slice(l.Layers, func(i, j int) bool { return l.Layers[i].ID < l.Layers[j].ID })
	for i := range l.Layers {
		files := l.Layers[i].Files
		sort.Slice(files, func(a, b int) bool {
			if files[a].Path != files[b].Path {
				return files[a].Path < files[b].Path
			}
			return files[a].Region < files[b].Region
		})
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := Path(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Layer returns the recorded state for a layer id.
func (l *Lock) Layer(id string) (*Layer, bool) {
	for i := range l.Layers {
		if l.Layers[i].ID == id {
			return &l.Layers[i], true
		}
	}
	return nil, false
}

// Put replaces the recorded state for a layer.
func (l *Lock) Put(entry Layer) {
	for i := range l.Layers {
		if l.Layers[i].ID == entry.ID {
			l.Layers[i] = entry
			return
		}
	}
	l.Layers = append(l.Layers, entry)
}

// Remove deletes the recorded state for a layer.
func (l *Lock) Remove(id string) bool {
	for i := range l.Layers {
		if l.Layers[i].ID == id {
			l.Layers = append(l.Layers[:i], l.Layers[i+1:]...)
			return true
		}
	}
	return false
}

// IDs lists the locked layer ids.
func (l *Lock) IDs() []string {
	ids := make([]string, 0, len(l.Layers))
	for _, e := range l.Layers {
		ids = append(ids, e.ID)
	}
	sort.Strings(ids)
	return ids
}

// Find looks up a recorded artifact.
//
// The owner is part of the key, not a detail: two layers may each own a region
// of the same name in the same file — `instructions` in AGENTS.md is the obvious
// case — and matching on path and region alone silently returns the wrong one.
func (l *Lock) Find(owner, path, region string) (Layer, File, bool) {
	for _, entry := range l.Layers {
		if entry.ID != owner {
			continue
		}
		for _, f := range entry.Files {
			if f.Path == path && f.Region == region {
				return entry, f, true
			}
		}
	}
	return Layer{}, File{}, false
}

// Hash is the digest ilk stores for provenance.
func Hash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}
