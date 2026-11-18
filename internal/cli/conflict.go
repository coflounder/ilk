package cli

import (
	"fmt"
	"strings"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/spf13/cobra"
)

// What to do with a file edited since ilk wrote it, when the layer wants to
// change it too. One flag with five values replaces four flags that could not be
// combined anyway.
const (
	conflictMerge     = "merge"      // three-way merge; refuse where the edits collide
	conflictMarkers   = "markers"    // merge, and write collisions as conflict markers
	conflictKeepMine  = "keep-mine"  // keep the file as it is, recorded as the new baseline
	conflictTakeLayer = "take-layer" // overwrite (or remove) the file with the layer's version
	conflictRefuse    = "refuse"     // do not merge; refuse every edited file
)

// conflictModes is the order the values are listed in help and errors.
var conflictModes = []string{conflictMerge, conflictMarkers, conflictKeepMine, conflictTakeLayer, conflictRefuse}

const onConflictUsage = "what to do with files edited since ilk wrote them, a `mode`: merge, markers, keep-mine, take-layer or refuse"

// addOnConflictFlag registers --on-conflict, defaulting to merge.
func addOnConflictFlag(cmd *cobra.Command, mode *string) {
	cmd.Flags().StringVar(mode, "on-conflict", conflictMerge, onConflictUsage)
}

// conflictOptions turns an --on-conflict value into plan options. Anything else
// is a usage error with a stable code, raised before any work is done.
func conflictOptions(mode string) (engine.PlanOptions, error) {
	switch mode {
	case conflictMerge, "":
		return engine.PlanOptions{}, nil
	case conflictMarkers:
		return engine.PlanOptions{MergeMarkers: true}, nil
	case conflictKeepMine:
		return engine.PlanOptions{Accept: true}, nil
	case conflictTakeLayer:
		return engine.PlanOptions{Force: true}, nil
	case conflictRefuse:
		return engine.PlanOptions{NoMerge: true}, nil
	}
	return engine.PlanOptions{}, &cliError{
		Exit:    exitUsage,
		Code:    "invalid_on_conflict",
		Message: fmt.Sprintf("--on-conflict %q is not a choice", mode),
		Fix:     "use one of " + strings.Join(conflictModes, ", "),
	}
}
