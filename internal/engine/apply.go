package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/coflounder/ilk/internal/targets"
)

// ModeDir marks a directory contract in the lockfile.
const ModeDir manifest.Mode = "dir"

// Apply executes a plan and rewrites the lockfile to match what is now on disk.
//
// Conflicts are never applied. They are reported and skipped, so a run that hits
// one still makes every safe change and leaves the user with a precise list of
// what it would not touch.
func (p *Project) Apply(pl *Plan) error {
	for i := range pl.Actions {
		a := &pl.Actions[i]
		if err := p.execute(a); err != nil {
			return fmt.Errorf("%s: %w", a.Path, err)
		}
	}
	return p.writeLock(pl)
}

func (p *Project) execute(a *Action) error {
	abs := p.Repo.Path(a.Path)

	switch a.Op {
	case OpConflict, OpUnchanged, OpSkip:
		return nil

	case OpMkdir:
		return os.MkdirAll(abs, 0o755)

	case OpRmdir:
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			// A directory that refuses to go is not worth failing a run over.
			a.Op = OpSkip
			a.Note = "could not remove: " + err.Error()
		}
		return nil

	case OpDelete:
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return err
		}
		pruneEmptyParents(p.Repo.Root, filepath.Dir(abs))
		return nil

	case OpChmod:
		return os.Chmod(abs, permFor(a.exec))

	case OpCreate, OpUpdate, OpRegionAdd, OpRegionUpdate, OpRegionRemove, OpVacate:
		if a.writeContent == nil {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		return writeAtomic(abs, *a.writeContent, permFor(a.exec))
	}
	return fmt.Errorf("unknown operation %q", a.Op)
}

func permFor(exec bool) os.FileMode {
	if exec {
		return 0o755
	}
	return 0o644
}

// writeLock rebuilds the lockfile from the actions that succeeded. Anything that
// conflicted keeps its previous entry, so a later run still knows what ilk
// believed it had written.
func (p *Project) writeLock(pl *Plan) error {
	next := lock.New()
	next.Targets = append([]string(nil), p.Config.Targets...)

	byOwner := map[string]*lock.Layer{}
	ensure := func(owner string) *lock.Layer {
		if e, ok := byOwner[owner]; ok {
			return e
		}
		e := &lock.Layer{ID: owner, Files: []lock.File{}}
		if l, ok := p.Layer(owner); ok {
			e.Version = l.Loaded.Manifest.Version
			e.Source = l.Loaded.Source
			e.Digest = l.Loaded.Digest
			e.Vars = l.Vars
		}
		byOwner[owner] = e
		return e
	}

	for _, a := range pl.Actions {
		if !a.track {
			continue
		}
		if a.Op == OpConflict {
			// Preserve what we knew before; the artifact is still ours in intent.
			if _, prev, ok := p.Lock.Find(a.Path, a.Region); ok {
				ensure(a.Owner).Files = append(ensure(a.Owner).Files, prev)
			}
			continue
		}
		entry := lock.File{
			Path:        a.Path,
			Mode:        a.Mode,
			Region:      a.Region,
			CreatedFile: a.createdFile,
			Owner:       a.Owner,
			Exec:        a.exec,
		}
		if a.Mode != manifest.ModeCreateOnly && a.Mode != ModeDir {
			entry.Hash = lock.Hash(a.hashBody)
		}
		e := ensure(a.Owner)
		e.Files = append(e.Files, entry)
	}

	for _, e := range byOwner {
		next.Put(*e)
	}
	return next.Save(p.Repo.Root)
}

// mergeFuncFor recovers the merge function for a co-owned file whose target is no
// longer configured, so `ilk drop` can still vacate it cleanly.
func (p *Project) mergeFuncFor(owner, path string) func(string, bool) (string, error) {
	name := strings.TrimPrefix(owner, TargetOwnerPrefix)
	if name == owner {
		return nil
	}
	t, err := targets.Get(name)
	if err != nil {
		return nil
	}
	in, err := p.targetInput()
	if err != nil {
		return nil
	}
	artifacts, err := t.Artifacts(in)
	if err != nil {
		return nil
	}
	for _, a := range artifacts {
		if a.Path == path && a.Merge != nil {
			return a.Merge
		}
	}
	return nil
}

// pruneEmptyParents removes directories left empty by a deletion, stopping at the
// repository root so ilk never walks out of the project.
func pruneEmptyParents(root, dir string) {
	for {
		if dir == root || !strings.HasPrefix(dir, root+string(os.PathSeparator)) {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func writeAtomic(path, content string, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".ilk-tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		return err
	}
	return os.Rename(name, path)
}
