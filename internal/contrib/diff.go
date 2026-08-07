package contrib

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/merge"
	"github.com/coflounder/ilk/internal/render"
)

// unified renders a change as a unified diff.
//
// It reuses the alignment the merge engine already has, rather than adding a
// second opinion about what changed to a codebase that has one. A proposal and an
// upgrade should agree about which lines moved.
func unified(path, before, after string) string {
	a := splitKeepingLines(before)
	b := splitKeepingLines(after)
	hunks := hunksFor(a, b, 3)
	if len(hunks) == 0 {
		return ""
	}

	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n", path)
	fmt.Fprintf(&out, "+++ b/%s\n", path)
	for _, h := range hunks {
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", h.aStart+1, h.aLen, h.bStart+1, h.bLen)
		out.WriteString(h.body)
	}
	return out.String()
}

type hunk struct {
	aStart, aLen int
	bStart, bLen int
	body         string
}

// hunksFor groups the changed regions with a few lines of context either side.
func hunksFor(a, b []string, context int) []hunk {
	changes := merge.Changes(a, b)
	if len(changes) == 0 {
		return nil
	}

	// Merge changes that sit close enough that their context would overlap,
	// because two hunks sharing lines read as a mistake.
	type span struct{ aLo, aHi, bLo, bHi int }
	var spans []span
	for _, c := range changes {
		s := span{aLo: c.ALo, aHi: c.AHi, bLo: c.BLo, bHi: c.BHi}
		if n := len(spans); n > 0 && s.aLo-spans[n-1].aHi <= context*2 {
			spans[n-1].aHi = s.aHi
			spans[n-1].bHi = s.bHi
			continue
		}
		spans = append(spans, s)
	}

	var out []hunk
	for _, s := range spans {
		aStart := max(0, s.aLo-context)
		aEnd := min(len(a), s.aHi+context)
		bStart := max(0, s.bLo-context)
		bEnd := min(len(b), s.bHi+context)

		var body strings.Builder
		for i := aStart; i < s.aLo; i++ {
			body.WriteString(" " + a[i] + "\n")
		}
		for i := s.aLo; i < s.aHi; i++ {
			body.WriteString("-" + a[i] + "\n")
		}
		for i := s.bLo; i < s.bHi; i++ {
			body.WriteString("+" + b[i] + "\n")
		}
		for i := s.aHi; i < aEnd; i++ {
			body.WriteString(" " + a[i] + "\n")
		}
		out = append(out, hunk{
			aStart: aStart, aLen: aEnd - aStart,
			bStart: bStart, bLen: bEnd - bStart,
			body: body.String(),
		})
	}
	return out
}

func splitKeepingLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	// A trailing newline produces an empty final element that is not a line.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

// renderPath renders a manifest destination through the layer's context.
func renderPath(l *engine.ResolvedLayer, path string) (string, error) {
	return render.Path(path, l.Ctx)
}

// readLayerFile reads a file from the layer's own tree, which is what a
// contribution ultimately has to change.
func readLayerFile(l *engine.ResolvedLayer, name string) (string, error) {
	data, err := fs.ReadFile(l.Loaded.FS, name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
