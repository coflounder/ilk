package engine

import (
	"fmt"

	"github.com/coflounder/ilk/internal/basestore"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/merge"
)

// resolution is what reconcile decided about an artifact that may have moved on
// either side.
type resolution struct {
	// Op is the operation to record.
	Op Op
	// Write is the content to put on disk. Empty means leave the file alone.
	Write string
	// Baseline is what ilk should expect to find next time.
	Baseline string
	// Delivered is the layer content this decision was made against — the common
	// ancestor for the next merge.
	Delivered string
	// Note explains the outcome in the plan.
	Note string
}

// reconcileArtifact decides what to do with one artifact given everything known
// about it: what is on disk, what the layer wants, and what ilk agreed last time.
//
// The three states that matter are whether the file still matches ilk's
// expectation, whether the layer has produced something new, and whether the file
// legitimately diverges from the layer because of an earlier merge or `--accept`.
// Conflating the last one with "unchanged" is how a tool silently destroys work.
func (p *Project) reconcileArtifact(current, incoming, owner string, locked lock.File, opts PlanOptions) resolution {
	delivered := locked.Delivered
	if delivered == "" {
		// A lockfile written before ancestors were tracked. Its Hash was both
		// things at once, which is exactly right for a file nobody had diverged.
		delivered = locked.Hash
	}

	fileAtBaseline := lock.Hash(current) == locked.Hash
	layerAtBaseline := lock.Hash(incoming) == delivered

	switch {
	case fileAtBaseline && layerAtBaseline:
		// Neither side has moved since ilk last looked. If the file differs from
		// the layer's output, that difference was agreed — leave it be.
		return resolution{Op: OpUnchanged, Baseline: current, Delivered: incoming}

	case fileAtBaseline && locked.Hash == delivered:
		// The file is exactly what the layer last produced, and the layer has
		// moved on. Nothing to preserve, so take the new version wholesale.
		return resolution{Op: OpUpdate, Write: incoming, Baseline: incoming, Delivered: incoming, Note: ""}

	case opts.Force:
		return resolution{Op: OpUpdate, Write: incoming, Baseline: incoming, Delivered: incoming}

	case opts.Accept:
		return resolution{
			Op:        OpAccept,
			Baseline:  current,
			Delivered: incoming,
			Note:      "kept your version and recorded it as ilk's baseline",
		}
	}

	return p.merge(delivered, current, incoming, owner, opts)
}

// merge attempts a three-way merge and turns the outcome into a resolution.
func (p *Project) merge(deliveredHash, local, incoming, owner string, opts PlanOptions) resolution {
	const edited = "edited since ilk wrote it"

	refuse := func(why string) resolution {
		return resolution{Op: OpConflict, Note: edited + why}
	}

	if opts.NoMerge {
		return refuse(" — merging is disabled; re-run with --force to discard your edits, or --accept to keep them")
	}

	base, found := basestore.Get(p.Repo.Root, deliveredHash)
	if !found {
		// No ancestor, no merge. This happens for artifacts written before the
		// base store existed, and after a clean that took .ilk/base with it.
		return refuse(" — ilk has no record of what it wrote, so it cannot merge; re-run with --accept to keep your version, or --force to take the layer's")
	}

	// Merging reconciles two changes. When the layer has nothing new to deliver,
	// the only difference is somebody's edit to a file ilk owns — that is drift,
	// and accepting it silently would hand ownership over without a decision.
	if base == incoming {
		return refuse(", and the layer has not changed — so there is nothing to merge. Re-run with --accept to keep your version and record it as the new baseline, with --force to restore ilk's, or move your edit outside the generated block")
	}

	result := merge.Three(base, local, incoming)

	switch {
	case result.Declined:
		return refuse(" — the file is too large to merge safely; re-run with --accept to keep your version, or --force to take the layer's")

	case result.Clean():
		return resolution{
			Op:        OpMerge,
			Write:     result.Merged,
			Baseline:  result.Merged,
			Delivered: incoming,
			Note:      fmt.Sprintf("merged %s's changes with yours", owner),
		}

	case opts.MergeMarkers:
		marked := merge.WithMarkers(base, local, incoming, "your version", owner)
		return resolution{
			Op:        OpMerge,
			Write:     marked,
			Baseline:  marked,
			Delivered: incoming,
			Note:      fmt.Sprintf("%s — written with conflict markers for you to resolve", result.Summarise()),
		}

	default:
		return resolution{Op: OpConflict, Note: fmt.Sprintf(
			"%s — resolve them by hand, re-run with --merge-markers to write both versions into the file, --accept to keep yours, or --force to take %s's",
			result.Summarise(), owner)}
	}
}
