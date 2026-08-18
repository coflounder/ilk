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
	OpMerge        Op = "merge"
	OpAccept       Op = "accept"
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
	// hashBody is what the lockfile records as ilk's expectation: the whole file
	// for a managed file, the block body for a fenced region.
	hashBody string
	// deliveredBody is the layer's own content behind this action, recorded as
	// the ancestor for the next merge.
	deliveredBody string
	exec          bool
	createdFile   bool
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
	// Baselines records, per layer, the files that were already present in the
	// directories that layer governs. See lock.Layer.Baseline.
	Baselines map[string][]string
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
	// NoBaseline governs a directory's existing contents from the moment the
	// layer is adopted, instead of exempting them. It is for a repository that
	// wants to be held to the new rules immediately.
	NoBaseline bool
	// NoMerge refuses any file that has been edited since ilk wrote it, instead
	// of attempting a three-way merge against what ilk last wrote.
	NoMerge bool
	// MergeMarkers writes conflicted regions with git-style markers rather than
	// leaving the file alone, for somebody who would rather resolve in place.
	MergeMarkers bool
	// Accept records what is on disk as ilk's new common ancestor, leaving the
	// content alone. It is how somebody says "my version is the truth now" after
	// resolving a conflict by hand — the counterpart to Force.
	Accept bool
}

// Plan computes what apply would do.
func (p *Project) Plan(opts PlanOptions) (*Plan, error) {
	desired, err := p.Desired()
	if err != nil {
		return nil, err
	}

	pl := &Plan{Baselines: map[string][]string{}}
	// files caches file content across actions so that several regions written
	// into one file see each other's results.
	files := newFileCache(p.Repo.Root)

	desiredKeys := map[string]bool{}
	for _, d := range desired {
		desiredKeys[d.key()] = true
	}

	if err := p.planBaselines(desired, pl, opts); err != nil {
		return nil, err
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
			"%s requires %s, which nothing supplies — set it under `capabilities:` in .ilk/config.yaml, or add a layer that provides it",
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

	_, locked, isLocked := p.Lock.Find(d.Owner, d.Path, d.Region)
	if isLocked {
		a.createdFile = locked.CreatedFile
	}

	// A symlink is settled before any content is read: reading through one would
	// follow it, and a link to a directory is not a file at all.
	if d.Mode == manifest.ModeSymlink {
		return p.planSymlink(a, d, locked, isLocked, opts)
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
		if isLocked {
			// ilk seeded this once and somebody deleted it. Seeding it again
			// would override a decision already made, and would do so on every
			// apply for ever.
			a.Op = OpSkip
			a.Note = "seeded once and since removed; not recreated"
			a.createdFile = locked.CreatedFile
			return a, nil
		}
		a.Op = OpCreate
		a.createdFile = true
		a.setWrite(d.Content)
		a.hashBody = ""
		a.deliveredBody = ""
		files.write(d.Path, d.Content)
		return a, nil

	case manifest.ModeManaged:
		if !exists {
			a.Op = OpCreate
			a.createdFile = true
			a.setWrite(d.Content)
			a.deliveredBody = d.Content
			files.write(d.Path, d.Content)
			return a, nil
		}
		if !isLocked {
			if opts.Force {
				a.Op = OpUpdate
				a.setWrite(d.Content)
				a.deliveredBody = d.Content
				files.write(d.Path, d.Content)
				return a, nil
			}
			a.Op = OpConflict
			a.Note = "exists but ilk did not write it — move it aside, or re-run with --force to let ilk take ownership"
			return a, nil
		}

		r := p.reconcileArtifact(current, d.Content, d.Owner, locked, opts)
		a.Op = r.Op
		a.Note = r.Note
		if r.Op == OpConflict {
			return a, nil
		}
		a.hashBody = r.Baseline
		a.deliveredBody = r.Delivered
		if r.Op == OpUnchanged {
			if execMismatch(p.Repo.Path(d.Path), d.Exec) {
				a.Op = OpChmod
			}
			return a, nil
		}
		if r.Write != "" || r.Op != OpAccept {
			a.setWrite(r.Write)
			files.write(d.Path, r.Write)
		}
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

		writeBody := func(op Op, note, newBody string) (Action, error) {
			updated, upErr := fence.Upsert(current, style, marker, newBody)
			if upErr != nil {
				return a, upErr
			}
			a.Op = op
			a.Note = note
			a.setWrite(updated)
			files.write(d.Path, updated)
			return a, nil
		}

		if !present {
			a.Op = OpRegionAdd
			a.deliveredBody = d.Content
			return writeBody(OpRegionAdd, "", d.Content)
		}
		if d.Mode == manifest.ModeAppendOnce {
			a.Op = OpUnchanged
			a.deliveredBody = d.Content
			a.hashBody = body
			return a, nil
		}
		if !isLocked {
			a.deliveredBody = d.Content
			if body == d.Content {
				return a, nil
			}
			return writeBody(OpRegionUpdate, "", d.Content)
		}

		r := p.reconcileArtifact(body, d.Content, d.Owner, locked, opts)
		a.Note = r.Note
		if r.Op == OpConflict {
			a.Op = OpConflict
			return a, nil
		}
		a.hashBody = r.Baseline
		a.deliveredBody = r.Delivered
		switch r.Op {
		case OpUnchanged, OpAccept:
			a.Op = r.Op
			return a, nil
		case OpUpdate:
			return writeBody(OpRegionUpdate, r.Note, r.Write)
		default:
			return writeBody(r.Op, r.Note, r.Write)
		}

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
		a.deliveredBody = merged
		if merged == current {
			// An absent file ilk has nothing to contribute to must also stay
			// out of the lockfile, or the drift check expects a file that was
			// deliberately never written.
			if !exists && strings.TrimSpace(merged) == "" {
				a.Op = OpSkip
				a.Note = "nothing to contribute"
				a.track = false
			}
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

// planSymlinkRemoval takes back a link ilk wrote, refusing one that has been
// repointed since — the same courtesy ModeManaged extends to an edited file.
func (p *Project) planSymlinkRemoval(a Action, f lock.File, opts PlanOptions) Action {
	abs := p.Repo.Path(f.Path)
	info, err := os.Lstat(abs)
	switch {
	case errors.Is(err, os.ErrNotExist):
		a.Op = OpSkip
		a.Note = "already gone"
		return a
	case err != nil:
		a.Op = OpSkip
		a.Note = "could not read: " + err.Error()
		return a
	case info.Mode()&os.ModeSymlink == 0:
		a.Op = OpSkip
		a.Note = "no longer a symlink — left alone"
		return a
	}
	target, err := os.Readlink(abs)
	if err != nil {
		a.Op = OpSkip
		a.Note = "could not read: " + err.Error()
		return a
	}
	if lock.Hash(target) != f.Hash && !opts.Force {
		a.Op = OpConflict
		a.Note = "repointed since ilk wrote it — left in place; delete it yourself, or re-run with --force"
		return a
	}
	a.Op = OpDelete
	return a
}

// planSymlink settles a link artifact. The rules mirror ModeManaged — ilk writes
// what it owns, refuses what somebody else put there, and never follows the link
// to reason about whatever is on the other end.
func (p *Project) planSymlink(a Action, d Desired, locked lock.File, isLocked bool, opts PlanOptions) (Action, error) {
	abs := p.Repo.Path(d.Path)
	a.hashBody = d.Content

	info, err := os.Lstat(abs)
	switch {
	case errors.Is(err, os.ErrNotExist):
		a.Op = OpCreate
		a.createdFile = true
		a.setWrite(d.Content)
		return a, nil
	case err != nil:
		return a, err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		// Something real occupies the path. Overwriting it would destroy content
		// ilk did not write, which --force is deliberately not a licence for here:
		// there is no ancestor to fall back on and nothing to merge.
		a.Op = OpConflict
		a.Note = "exists and is not a symlink — move it aside; ilk will not replace a real file with a link"
		return a, nil
	}

	target, err := os.Readlink(abs)
	if err != nil {
		return a, err
	}
	if target == d.Content {
		a.Op = OpUnchanged
		return a, nil
	}
	if !isLocked && !opts.Force {
		a.Op = OpConflict
		a.Note = "a symlink ilk did not write is already here — remove it, or re-run with --force to let ilk take ownership"
		return a, nil
	}
	if isLocked && lock.Hash(target) != locked.Hash && !opts.Force {
		a.Op = OpConflict
		a.Note = "repointed since ilk wrote it — left alone; re-run with --force to reset it"
		return a, nil
	}
	a.Op = OpUpdate
	a.setWrite(d.Content)
	return a, nil
}

// planRemovals reverses artifacts recorded in the lockfile that nothing wants any
// more — the rm half of the contract.
func (p *Project) planRemovals(desiredKeys map[string]bool, files *fileCache, opts PlanOptions) ([]Action, error) {
	var out []Action

	// Reverse order so regions come out of a file before the file itself is
	// considered for deletion, and children before parents.
	type entry struct {
		owner string
		file  lock.File
	}
	var entries []entry
	for _, l := range p.Lock.Owners {
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

		// As in planOne, a link is settled without reading through it.
		if e.file.Mode == manifest.ModeSymlink {
			out = append(out, p.planSymlinkRemoval(a, e.file, opts))
			continue
		}

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
	for _, l := range p.Lock.Owners {
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
