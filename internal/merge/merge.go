package merge

import (
	"fmt"
	"strings"
)

// Conflict is a region the two sides changed incompatibly.
type Conflict struct {
	// Line is the 1-indexed line in the merged output where the region begins.
	Line int
	// Local is what is on disk — the user's version.
	Local []string
	// Incoming is what the layer wants.
	Incoming []string
}

// Result is the outcome of a three-way merge.
type Result struct {
	// Merged is the combined content. It is only meaningful when Clean reports
	// true; where a region conflicted it holds the local side, so that reported
	// line numbers stay useful.
	Merged string
	// Conflicts lists regions that could not be resolved.
	Conflicts []Conflict
	// Declined is set when the inputs were too large to align. Callers must fall
	// back to refusing the change rather than treating this as a clean merge.
	Declined bool
}

// Clean reports whether the merge fully succeeded.
func (r Result) Clean() bool { return !r.Declined && len(r.Conflicts) == 0 }

// change is one side's edit to a half-open range of base lines. An insertion has
// start == end: it replaces nothing and sits between two base lines.
type change struct {
	start, end int
	repl       []string
}

func (c change) isInsertion() bool { return c.start == c.end }

// segment is a piece of the merged output: either resolved content or a region
// where the two sides disagree.
type segment struct {
	lines    []string
	conflict bool
	local    []string
	incoming []string
}

// diffChanges expresses `other` as a set of edits to `base`.
func diffChanges(base, other []string) ([]change, bool) {
	matches, ok := align(base, other)
	if !ok {
		return nil, false
	}
	var out []change
	pa, pb := 0, 0
	for _, m := range matches {
		if m.a > pa || m.b > pb {
			out = append(out, change{start: pa, end: m.a, repl: other[pb:m.b]})
		}
		pa, pb = m.a+1, m.b+1
	}
	if pa < len(base) || pb < len(other) {
		out = append(out, change{start: pa, end: len(base), repl: other[pb:]})
	}
	return out, true
}

// merge3 walks the base, applying each side's edits and flagging the places
// where they collide.
//
// Two edits collide only when their base ranges genuinely overlap. An edit to a
// line and an insertion next to that line are independent, which is the common
// shape when a layer appends a section and a user has tweaked the line above it.
func merge3(base, local, incoming []string) ([]segment, bool) {
	localChanges, ok := diffChanges(base, local)
	if !ok {
		return nil, false
	}
	incomingChanges, ok := diffChanges(base, incoming)
	if !ok {
		return nil, false
	}

	var segments []segment
	emit := func(lines ...string) {
		if len(lines) == 0 {
			return
		}
		if n := len(segments); n > 0 && !segments[n-1].conflict {
			segments[n-1].lines = append(segments[n-1].lines, lines...)
			return
		}
		segments = append(segments, segment{lines: append([]string(nil), lines...)})
	}

	li, ii, i := 0, 0, 0
	for {
		var l, in *change
		if li < len(localChanges) && localChanges[li].start <= i {
			l = &localChanges[li]
		}
		if ii < len(incomingChanges) && incomingChanges[ii].start <= i {
			in = &incomingChanges[ii]
		}

		switch {
		case l == nil && in == nil:
			if i >= len(base) {
				return segments, true
			}
			emit(base[i])
			i++

		case in == nil:
			emit(l.repl...)
			i = maxInt(i, l.end)
			li++

		case l == nil:
			emit(in.repl...)
			i = maxInt(i, in.end)
			ii++

		case l.isInsertion() && in.isInsertion():
			// Both sides inserted at the same point. Identical insertions are the
			// same idea arrived at twice; different ones have no defensible order.
			if equal(l.repl, in.repl) {
				emit(l.repl...)
			} else {
				segments = append(segments, segment{
					conflict: true,
					local:    append([]string(nil), l.repl...),
					incoming: append([]string(nil), in.repl...),
				})
			}
			li++
			ii++

		case !overlaps(*l, *in):
			// An insertion sitting beside the other side's edit. Apply the
			// insertion and reconsider; the ranged edit is handled next time round.
			if l.isInsertion() {
				emit(l.repl...)
				li++
			} else {
				emit(in.repl...)
				ii++
			}

		case sameEdit(*l, *in):
			emit(l.repl...)
			i = maxInt(i, l.end)
			li++
			ii++

		default:
			// A genuine collision. Widen the region until no further edit on
			// either side reaches into it, then render both sides in full so the
			// report shows what each actually wanted.
			start, end := minInt(l.start, in.start), maxInt(l.end, in.end)
			localSet := []change{*l}
			incomingSet := []change{*in}
			li++
			ii++

			// Absorb any further edit that reaches into the region, widening it
			// until nothing else does. Reporting half a collision would send the
			// reader to fix something that is only part of the problem.
			for {
				grew := false
				if li < len(localChanges) && touches(localChanges[li], start, end) {
					c := localChanges[li]
					localSet = append(localSet, c)
					li++
					start, end, grew = minInt(start, c.start), maxInt(end, c.end), true
				}
				if ii < len(incomingChanges) && touches(incomingChanges[ii], start, end) {
					c := incomingChanges[ii]
					incomingSet = append(incomingSet, c)
					ii++
					start, end, grew = minInt(start, c.start), maxInt(end, c.end), true
				}
				if !grew {
					break
				}
			}

			segments = append(segments, segment{
				conflict: true,
				local:    renderSide(base, localSet, start, end),
				incoming: renderSide(base, incomingSet, start, end),
			})
			i = maxInt(i, end)
		}
	}
}

// overlaps reports whether two edits contend for the same base lines.
func overlaps(a, b change) bool {
	return a.start < b.end && b.start < a.end
}

// touches reports whether an edit reaches into an already-conflicted region. A
// ranged edit counts when its lines intersect; an insertion counts only when it
// sits strictly inside, since one abutting the boundary is independent.
func touches(c change, start, end int) bool {
	if c.isInsertion() {
		return c.start > start && c.start < end
	}
	return c.start < end && c.end > start
}

func sameEdit(a, b change) bool {
	return a.start == b.start && a.end == b.end && equal(a.repl, b.repl)
}

// renderSide reconstructs one side's version of a base range.
func renderSide(base []string, changes []change, start, end int) []string {
	var out []string
	i := start
	for _, c := range changes {
		for ; i < c.start && i < end; i++ {
			out = append(out, base[i])
		}
		out = append(out, c.repl...)
		if c.end > i {
			i = c.end
		}
	}
	for ; i < end; i++ {
		out = append(out, base[i])
	}
	return out
}

// Three performs a three-way merge.
//
// base is what ilk last wrote, local is what is on disk now, and incoming is what
// the layer wants the file to become. Where only one side moved away from base,
// that side wins. Where both moved identically, the shared result wins. Anything
// else is a conflict, never a guess.
func Three(base, local, incoming string) Result {
	segments, ok := merge3(splitLines(base), splitLines(local), splitLines(incoming))
	if !ok {
		return Result{Declined: true}
	}

	var out []string
	var conflicts []Conflict
	for _, s := range segments {
		if !s.conflict {
			out = append(out, s.lines...)
			continue
		}
		conflicts = append(conflicts, Conflict{
			Line:     len(out) + 1,
			Local:    s.local,
			Incoming: s.incoming,
		})
		// Keep the user's version provisionally. This is never written while
		// conflicts exist; it keeps the reported line numbers meaningful.
		out = append(out, s.local...)
	}
	return Result{Merged: join(out), Conflicts: conflicts}
}

// WithMarkers renders the merge with git-style conflict markers, for a user who
// would rather resolve the collision in the file than out of band.
func WithMarkers(base, local, incoming, localLabel, incomingLabel string) string {
	segments, ok := merge3(splitLines(base), splitLines(local), splitLines(incoming))
	if !ok {
		return local
	}
	var out []string
	for _, s := range segments {
		if !s.conflict {
			out = append(out, s.lines...)
			continue
		}
		out = append(out, ConflictMarker+localLabel)
		out = append(out, s.local...)
		out = append(out, "=======")
		out = append(out, s.incoming...)
		out = append(out, ">>>>>>> "+incomingLabel)
	}
	return join(out)
}

// ConflictMarker is the token `ilk check` looks for to find files somebody left
// half-resolved.
const ConflictMarker = "<<<<<<< "

// Summarise renders conflicts as a short, actionable sentence.
func (r Result) Summarise() string {
	if len(r.Conflicts) == 0 {
		return ""
	}
	lines := make([]string, 0, len(r.Conflicts))
	for _, c := range r.Conflicts {
		lines = append(lines, fmt.Sprint(c.Line))
	}
	noun := "hunk"
	if len(r.Conflicts) > 1 {
		noun = "hunks"
	}
	return fmt.Sprintf("%d %s conflict with your edits, around line %s",
		len(r.Conflicts), noun, strings.Join(lines, ", "))
}

// splitLines splits content into lines without inventing a trailing newline;
// join is its inverse.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func join(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
