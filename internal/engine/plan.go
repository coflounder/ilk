package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/fence"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/manifest"
)

// Op is what apply will do to one artifact.
type Op string

const (
	OpMkdir        Op = "mkdir"
	OpCreate       Op = "create"
	OpUpdate       Op = "update"
	OpRegionAdd    Op = "region-add"
	OpRegionUpdate Op = "region-update"
	OpRegionRemove Op = "region-remove"
	OpDelete       Op = "delete"
	OpVacate       Op = "vacate"
	OpRmdir        Op = "rmdir"
	OpChmod        Op = "chmod"
	OpUnchanged    Op = "unchanged"
	OpSkip         Op = "skip"
	OpConflict     Op = "conflict"
)

// Changes reports whether the op modifies the repository.
func (o Op) Changes() bool {
	switch o {
	case OpUnchanged, OpSkip, OpConflict:
		return false
	}
	return true
}

// Action is one planned operation.
type Action struct {
	Op     Op
	Path   string
	Region string
	Owner  string
	Mode   manifest.Mode
	Note   string

	// writeContent is the full file content apply should write. It is nil when
	// the action writes nothing.
	writeContent *string
	// hashBody is what the lockfile records as ilk's output: the whole file for
	// a managed file, the block body for a fenced region.
	hashBody    string
	exec        bool
	createdFile bool
	// track records the artifact in the new lockfile even when nothing changed.
	track bool
	// Removal marks an action that reverses something ilk previously wrote. A
	// skipped removal is worth showing — "left in place" is exactly what somebody
	// dropping a layer wants to know.
	Removal bool
}

// setWrite marks the content apply should write to disk.
func (a *Action) setWrite(s string) { a.writeContent = &s }

// Plan is the full reconciliation, ready to be shown before anything happens.
type Plan struct {
	Actions  []Action
	Warnings []string
}

// Changes returns only the actions that would modify the repository.
func (p *Plan) Changes() []Action {
	var out []Action
	for _, a := range p.Actions {
		if a.Op.Changes() {
			out = append(out, a)
		}
	}
	return out
}

// Conflicts returns the actions blocked because a human edited ilk's output.
func (p *Plan) Conflicts() []Action {
	var out []Action
	for _, a := range p.Actions {
		if a.Op == OpConflict {
			out = append(out, a)
		}
	}
	return out
}

// Empty reports whether applying this plan would do nothing.
func (p *Plan) Empty() bool { return len(p.Changes()) == 0 }

// PlanOptions tunes reconciliation.
type PlanOptions struct {
	// Force overwrites files a human has edited since ilk wrote them. It is the
	// escape hatch for "yes, I know, take it back".
	Force bool
	// Prune removes artifacts recorded in the lockfile that are no longer
	// desired. It is on for every command except a targeted adopt, where
	// removing another layer's files would be a surprise.
	Prune bool
}

// Plan computes what apply would do.
func (p *Project) Plan(opts PlanOptions) (*Plan, error) {
	desired, err := p.Desired()
	if err != nil {
		return nil, err
	}

	pl := &Plan{}
	// files caches file content across actions so that several regions written
	// into one file see each other's results.
	files := newFileCache(p.Repo.Root)

	desiredKeys := map[string]bool{}
	for _, d := range desired {
		desiredKeys[d.key()] = true
	}

	for _, d := range desired {
		action, err := p.planOne(d, files, opts)
		if err != nil {
			return nil, err
		}
		pl.Actions = append(pl.Actions, action)
	}

	if opts.Prune {
		removals, err := p.planRemovals(desiredKeys, files, opts)
		if err != nil {
			return nil, err
		}
		pl.Actions = append(pl.Actions, removals...)
	}

	for id, missing := range p.MissingRequirements() {
		pl.Warnings = append(pl.Warnings, fmt.Sprintf(
			"%s requires %s, which nothing supplies — set it under `capabilities:` in .ilk/config.yaml, or adopt a layer that provides it",
			id, strings.Join(missing, ", ")))
	}
	sort.Strings(pl.Warnings)

	return pl, nil
}

func (p *Project) planOne(d Desired, files *fileCache, opts PlanOptions) (Action, error) {
	a := Action{
		Op: OpUnchanged, Path: d.Path, Region: d.Region, Owner: d.Owner,
		Mode: d.Mode, hashBody: d.Content, exec: d.Exec, track: true,
	}

	if d.Dir {
		a.Mode = ModeDir
		info, err := os.Stat(p.Repo.Path(d.Path))
		switch {
		case errors.Is(err, os.ErrNotExist):
			a.Op = OpMkdir
		case err != nil:
			return a, err
		case !info.IsDir():
			a.Op = OpConflict
			a.Note = "exists and is not a directory"
		}
		return a, nil
	}

	_, locked, isLocked := p.Lock.Find(d.Path, d.Region)
	if isLocked {
		a.createdFile = locked.CreatedFile
	}

	current, exists, err := files.read(d.Path)
	if err != nil {
		return a, err
	}

	switch d.Mode {
	case manifest.ModeCreateOnly:
		if exists {
			a.Op = OpSkip
			a.Note = "already present; ilk seeds this file once and never touches it again"
			return a, nil
		}
		a.Op = OpCreate
		a.createdFile = true
		a.setWrite(d.Content)
		a.hashBody = ""
		files.write(d.Path, d.Content)
		return a, nil

	case manifest.ModeManaged:
		if !exists {
			a.Op = OpCreate
			a.createdFile = true
			a.setWrite(d.Content)
			files.write(d.Path, d.Content)
			return a, nil
		}
		if current == d.Content {
			if execMismatch(p.Repo.Path(d.Path), d.Exec) {
				a.Op = OpChmod
			}
			return a, nil
		}
		if !isLocked {
			if opts.Force {
				a.Op = OpUpdate
				a.setWrite(d.Content)
				files.write(d.Path, d.Content)
				return a, nil
			}
			a.Op = OpConflict
			a.Note = "exists but ilk did not write it — move it aside, or re-run with --force to let ilk take ownership"
			return a, nil
		}
		if lock.Hash(current) != locked.Hash && !opts.Force {
			a.Op = OpConflict
			a.Note = "edited since ilk wrote it — your changes would be lost; re-run with --force to discard them"
			return a, nil
		}
		a.Op = OpUpdate
		a.setWrite(d.Content)
		files.write(d.Path, d.Content)
		return a, nil

	case manifest.ModeRegion, manifest.ModeAppendOnce:
		style := fence.StyleFor(d.Path)
		marker := fence.Marker{Layer: d.Owner, Region: d.Region}
		body, present, err := fence.Extract(current, style, marker)
		if err != nil {
			a.Op = OpConflict
			a.Note = err.Error()
			return a, nil
		}
		if !exists {
			a.createdFile = true
		}
		if present {
			if d.Mode == manifest.ModeAppendOnce {
				a.Op = OpUnchanged
				return a, nil
			}
			if body == d.Content {
				return a, nil
			}
			if isLocked && lock.Hash(body) != locked.Hash && !opts.Force {
				a.Op = OpConflict
				a.Note = "the block was edited by hand — your changes would be lost; re-run with --force to discard them"
				return a, nil
			}
			a.Op = OpRegionUpdate
		} else {
			a.Op = OpRegionAdd
		}
		updated, err := fence.Upsert(current, style, marker, d.Content)
		if err != nil {
			return a, err
		}
		a.setWrite(updated)
		files.write(d.Path, updated)
		return a, nil

	case manifest.ModeMerge:
		merged, err := d.Merge(current, true)
		if err != nil {
			a.Op = OpConflict
			a.Note = err.Error()
			return a, nil
		}
		// The lockfile records the merged result, not the artifact's (empty)
		// content, so that a co-owned file is not reported as drifted the moment
		// somebody else legitimately edits a different part of it.
		a.hashBody = merged
		if merged == current {
			return a, nil
		}
		if !exists {
			a.Op = OpCreate
			a.createdFile = true
		} else {
			a.Op = OpUpdate
		}
		if strings.TrimSpace(merged) == "" {
			// Nothing of ilk's left. If ilk created the file it is a husk, so
			// remove it; if it was already there, leave the empty file alone
			// rather than deleting something ilk did not create.
			if !exists {
				a.Op = OpSkip
				a.Note = "nothing to contribute"
				a.track = false
				return a, nil
			}
			if a.createdFile {
				a.Op = OpDelete
				a.Note = "created by ilk and now empty"
				a.track = false
				files.remove(d.Path)
				return a, nil
			}
		}
		a.setWrite(merged)
		files.write(d.Path, merged)
		return a, nil
	}

	return a, fmt.Errorf("%s: unsupported mode %q", d.Path, d.Mode)
}

// planRemovals reverses artifacts recorded in the lockfile that nothing wants any
// more — the drop half of the contract.
func (p *Project) planRemovals(desiredKeys map[string]bool, files *fileCache, opts PlanOptions) ([]Action, error) {
	var out []Action

	// Reverse order so regions come out of a file before the file itself is
	// considered for deletion, and children before parents.
	type entry struct {
		owner string
		file  lock.File
	}
	var entries []entry
	for _, l := range p.Lock.Layers {
		for _, f := range l.Files {
			entries = append(entries, entry{l.ID, f})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].file.Path != entries[j].file.Path {
			return entries[i].file.Path > entries[j].file.Path
		}
		return entries[i].file.Region > entries[j].file.Region
	})

	for _, e := range entries {
		key := e.owner + "\x00" + e.file.Path + "\x00" + e.file.Region
		if desiredKeys[key] {
			continue
		}
		if e.file.Mode == ModeDir {
			continue // handled by planDirRemovals, which knows not to read a directory as a file
		}
		a := Action{Path: e.file.Path, Region: e.file.Region, Owner: e.owner, Mode: e.file.Mode, Removal: true}
		current, exists, err := files.read(e.file.Path)
		if err != nil {
			return nil, err
		}

		switch e.file.Mode {
		case manifest.ModeCreateOnly:
			a.Op = OpSkip
			a.Note = "seeded by ilk and yours now — left in place"

		case manifest.ModeManaged:
			if !exists {
				a.Op = OpSkip
				a.Note = "already gone"
				break
			}
			if lock.Hash(current) != e.file.Hash && !opts.Force {
				a.Op = OpConflict
				a.Note = "edited since ilk wrote it — left in place; delete it yourself, or re-run with --force"
				break
			}
			a.Op = OpDelete
			files.remove(e.file.Path)

		case manifest.ModeRegion, manifest.ModeAppendOnce:
			if !exists {
				a.Op = OpSkip
				a.Note = "already gone"
				break
			}
			style := fence.StyleFor(e.file.Path)
			marker := fence.Marker{Layer: e.owner, Region: e.file.Region}
			body, present, err := fence.Extract(current, style, marker)
			if err != nil {
				a.Op = OpConflict
				a.Note = err.Error()
				break
			}
			if !present {
				a.Op = OpSkip
				a.Note = "block already gone"
				break
			}
			if e.file.Hash != "" && lock.Hash(body) != e.file.Hash && !opts.Force {
				a.Op = OpConflict
				a.Note = "the block was edited by hand — left in place; re-run with --force to remove it anyway"
				break
			}
			updated, _, err := fence.Remove(current, style, marker)
			if err != nil {
				return nil, err
			}
			a.Op = OpRegionRemove
			// A file ilk created that now holds nothing but whitespace is a husk;
			// remove it rather than leave litter behind.
			if e.file.CreatedFile && strings.TrimSpace(updated) == "" {
				a.Op = OpDelete
				a.Note = "created by ilk and now empty"
				files.remove(e.file.Path)
			} else {
				files.write(e.file.Path, updated)
				a.setWrite(updated)
			}

		case manifest.ModeMerge:
			// A co-owned file is vacated, never deleted: ilk strips its own
			// entries and leaves whatever else the user put there.
			merge := p.mergeFuncFor(e.owner, e.file.Path)
			if merge == nil || !exists {
				a.Op = OpSkip
				a.Note = "nothing of ilk's left in this file"
				break
			}
			vacated, err := merge(current, false)
			if err != nil {
				a.Op = OpConflict
				a.Note = err.Error()
				break
			}
			if vacated == current {
				a.Op = OpSkip
				a.Note = "nothing of ilk's left in this file"
				break
			}
			a.Op = OpVacate
			if strings.TrimSpace(vacated) == "" && e.file.CreatedFile {
				a.Op = OpDelete
				a.Note = "created by ilk and now empty"
				files.remove(e.file.Path)
				break
			}
			a.setWrite(vacated)
			files.write(e.file.Path, vacated)

		default:
			a.Op = OpSkip
		}
		out = append(out, a)
	}

	// Directories ilk created that are now empty.
	out = append(out, p.planDirRemovals(desiredKeys)...)
	return out, nil
}

func (p *Project) planDirRemovals(desiredKeys map[string]bool) []Action {
	var out []Action
	seen := map[string]bool{}
	for _, l := range p.Lock.Layers {
		for _, f := range l.Files {
			if f.Mode != ModeDir || f.Path == "" {
				continue
			}
			key := l.ID + "\x00" + f.Path + "\x00"
			if desiredKeys[key] || seen[f.Path] {
				continue
			}
			seen[f.Path] = true
			a := Action{Op: OpRmdir, Path: f.Path, Owner: l.ID, Removal: true}
			if !dirIsEmpty(p.Repo.Path(f.Path)) {
				a.Op = OpSkip
				a.Note = "not empty — left in place"
			}
			out = append(out, a)
		}
	}
	return out
}

func dirIsEmpty(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	return len(entries) == 0
}

func execMismatch(path string, want bool) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	isExec := info.Mode().Perm()&0o100 != 0
	return isExec != want
}

// fileCache holds pending content so that several actions touching one file
// compose correctly during planning, without writing anything to disk.
type fileCache struct {
	root    string
	content map[string]string
	deleted map[string]bool
	known   map[string]bool
}

func newFileCache(root string) *fileCache {
	return &fileCache{root: root, content: map[string]string{}, deleted: map[string]bool{}, known: map[string]bool{}}
}

func (c *fileCache) read(rel string) (string, bool, error) {
	if c.deleted[rel] {
		return "", false, nil
	}
	if v, ok := c.content[rel]; ok {
		return v, c.known[rel], nil
	}
	data, err := os.ReadFile(filepath.Join(c.root, rel))
	if errors.Is(err, os.ErrNotExist) {
		c.content[rel] = ""
		c.known[rel] = false
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	c.content[rel] = string(data)
	c.known[rel] = true
	return string(data), true, nil
}

func (c *fileCache) write(rel, content string) {
	c.content[rel] = content
	c.known[rel] = true
	delete(c.deleted, rel)
}

func (c *fileCache) remove(rel string) {
	c.deleted[rel] = true
	delete(c.content, rel)
	delete(c.known, rel)
}
