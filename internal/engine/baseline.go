package engine

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A layer governs what happens next, not what came before.
//
// When a layer's directory contract lands on a directory that already exists and
// already has files in it — the common case for `docs/` — those files are
// recorded as that layer's baseline and exempted from its checks. Without this,
// `ilk init` in any repository with existing documentation greets the user with a
// wall of failures about files nobody has touched, which is a good way to get the
// tool deleted.
//
// The exemption is a ratchet, not an amnesty: it is recorded in the lockfile,
// visible in `ilk status`, and `ilk baseline clear` brings a file under the rules
// whenever somebody is ready to conform it.

// planBaselines records a baseline for any layer that is newly governing a
// directory which already has contents.
func (p *Project) planBaselines(desired []Desired, pl *Plan, opts PlanOptions) error {
	for _, l := range p.Layers {
		id := l.ID()

		// A layer that has been applied here before already has its answer; the
		// baseline is decided once, when the layer arrives.
		if entry, ok := p.Lock.Layer(id); ok {
			if len(entry.Baseline) > 0 {
				pl.Baselines[id] = entry.Baseline
			}
			continue
		}
		if opts.NoBaseline {
			continue
		}

		var existing []string
		for _, d := range desired {
			if !d.Dir || d.Owner != id {
				continue
			}
			found, err := existingFiles(p.Repo.Path(d.Path), d.Path)
			if err != nil {
				return err
			}
			existing = append(existing, found...)
		}
		if len(existing) == 0 {
			continue
		}
		sort.Strings(existing)
		pl.Baselines[id] = existing
	}
	return nil
}

// existingFiles lists the files already in a directory, relative to the
// repository root. Dot-directories are skipped: nothing in them is what a record
// check is looking at.
func existingFiles(abs, rel string) ([]string, error) {
	info, err := os.Stat(abs)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	var out []string
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != abs && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		sub, relErr := filepath.Rel(abs, path)
		if relErr != nil {
			return relErr
		}
		out = append(out, filepath.ToSlash(filepath.Join(rel, sub)))
		return nil
	})
	return out, err
}

// Baseline returns every path currently exempt from its layer's checks.
func (p *Project) Baseline() map[string]bool {
	out := map[string]bool{}
	for _, entry := range p.Lock.Layers {
		for _, path := range entry.Baseline {
			out[path] = true
		}
	}
	return out
}

// BaselineByLayer returns the exempt paths grouped by the layer that governs
// them, for `ilk baseline list` and `ilk status`.
func (p *Project) BaselineByLayer() map[string][]string {
	out := map[string][]string{}
	for _, entry := range p.Lock.Layers {
		if len(entry.Baseline) > 0 {
			out[entry.ID] = append([]string(nil), entry.Baseline...)
		}
	}
	return out
}

// ClearBaseline removes paths from the exemption so that the governing layer's
// checks start applying to them. Passing no paths clears everything.
//
// It reports which paths were actually cleared, so the caller can tell the
// difference between "done" and "that file was not exempt in the first place".
func (p *Project) ClearBaseline(paths []string) []string {
	wanted := map[string]bool{}
	for _, path := range paths {
		wanted[filepath.ToSlash(filepath.Clean(path))] = true
	}

	var cleared []string
	for i := range p.Lock.Layers {
		entry := &p.Lock.Layers[i]
		if len(entry.Baseline) == 0 {
			continue
		}
		kept := entry.Baseline[:0:0]
		for _, path := range entry.Baseline {
			if len(wanted) == 0 || wanted[path] {
				cleared = append(cleared, path)
				continue
			}
			kept = append(kept, path)
		}
		entry.Baseline = kept
	}
	sort.Strings(cleared)
	return cleared
}
