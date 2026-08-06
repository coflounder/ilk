package merge

import (
	"fmt"
	"strings"
	"testing"
)

// The contract: a clean merge must preserve every edit both sides made, and
// anything ambiguous must be reported rather than resolved by guesswork. These
// tests are the reason to trust the engine writing to a file somebody edited.

func lines(s ...string) string {
	if len(s) == 0 {
		return ""
	}
	return strings.Join(s, "\n") + "\n"
}

func TestDisjointEditsMergeCleanly(t *testing.T) {
	base := lines("one", "two", "three", "four", "five")
	local := lines("one", "TWO", "three", "four", "five")    // user changed line 2
	incoming := lines("one", "two", "three", "four", "FIVE") // layer changed line 5

	got := Three(base, local, incoming)
	if !got.Clean() {
		t.Fatalf("expected a clean merge, got %+v", got.Conflicts)
	}
	want := lines("one", "TWO", "three", "four", "FIVE")
	if got.Merged != want {
		t.Fatalf("got  %q\nwant %q", got.Merged, want)
	}
}

func TestOnlyIncomingChangedTakesIncoming(t *testing.T) {
	base := lines("a", "b", "c")
	local := lines("a", "b", "c")
	incoming := lines("a", "B", "c")

	got := Three(base, local, incoming)
	if !got.Clean() || got.Merged != incoming {
		t.Fatalf("got %q clean=%v", got.Merged, got.Clean())
	}
}

func TestOnlyLocalChangedKeepsLocal(t *testing.T) {
	base := lines("a", "b", "c")
	local := lines("a", "USER", "c")
	incoming := lines("a", "b", "c")

	got := Three(base, local, incoming)
	if !got.Clean() || got.Merged != local {
		t.Fatalf("got %q clean=%v", got.Merged, got.Clean())
	}
}

func TestIdenticalChangesOnBothSidesAreNotAConflict(t *testing.T) {
	base := lines("a", "b", "c")
	same := lines("a", "SAME", "c")

	got := Three(base, same, same)
	if !got.Clean() {
		t.Fatalf("two sides making the same change is not a conflict: %+v", got.Conflicts)
	}
	if got.Merged != same {
		t.Fatalf("got %q want %q", got.Merged, same)
	}
}

func TestOverlappingEditsConflict(t *testing.T) {
	base := lines("a", "b", "c")
	local := lines("a", "USER", "c")
	incoming := lines("a", "LAYER", "c")

	got := Three(base, local, incoming)
	if got.Clean() {
		t.Fatal("both sides rewrote the same line; that must not merge silently")
	}
	if len(got.Conflicts) != 1 {
		t.Fatalf("expected one conflict, got %d", len(got.Conflicts))
	}
	c := got.Conflicts[0]
	if !equal(c.Local, []string{"USER"}) || !equal(c.Incoming, []string{"LAYER"}) {
		t.Fatalf("conflict reported the wrong content: %+v", c)
	}
	if c.Line != 2 {
		t.Errorf("conflict should be reported at line 2, got %d", c.Line)
	}
}

func TestInsertionsOnBothSidesAtDifferentPlaces(t *testing.T) {
	base := lines("header", "body", "footer")
	local := lines("header", "user note", "body", "footer")
	incoming := lines("header", "body", "layer note", "footer")

	got := Three(base, local, incoming)
	if !got.Clean() {
		t.Fatalf("expected clean, got %+v", got.Conflicts)
	}
	if !strings.Contains(got.Merged, "user note") || !strings.Contains(got.Merged, "layer note") {
		t.Fatalf("both insertions must survive: %q", got.Merged)
	}
}

func TestAppendOnBothSidesConflicts(t *testing.T) {
	// Both sides appending different content at the same point is genuinely
	// ambiguous — there is no way to know the intended order.
	base := lines("a")
	local := lines("a", "user tail")
	incoming := lines("a", "layer tail")

	got := Three(base, local, incoming)
	if got.Clean() {
		t.Fatalf("competing appends should conflict, got %q", got.Merged)
	}
}

func TestDeletionOnOneSideAndEditOnTheOtherConflicts(t *testing.T) {
	base := lines("a", "b", "c")
	local := lines("a", "c")          // user deleted b
	incoming := lines("a", "B!", "c") // layer rewrote b

	got := Three(base, local, incoming)
	if got.Clean() {
		t.Fatalf("delete-versus-edit is a conflict, got %q", got.Merged)
	}
}

func TestDeletionOnBothSidesIsClean(t *testing.T) {
	base := lines("a", "b", "c")
	local := lines("a", "c")
	incoming := lines("a", "c")

	got := Three(base, local, incoming)
	if !got.Clean() || got.Merged != lines("a", "c") {
		t.Fatalf("got %q clean=%v", got.Merged, got.Clean())
	}
}

func TestLargeFileWithSmallDisjointEdits(t *testing.T) {
	// The realistic shape: a long generated document, one section rewritten by
	// the layer and an unrelated paragraph edited by the user.
	var b []string
	for i := 0; i < 400; i++ {
		b = append(b, fmt.Sprintf("line %d", i))
	}
	base := lines(b...)

	l := append([]string(nil), b...)
	l[10] = "line 10 — edited by the user"
	local := lines(l...)

	in := append([]string(nil), b...)
	in[300] = "line 300 — rewritten by the layer"
	incoming := lines(in...)

	got := Three(base, local, incoming)
	if !got.Clean() {
		t.Fatalf("expected clean, got %+v", got.Conflicts)
	}
	if !strings.Contains(got.Merged, "edited by the user") {
		t.Error("the user's edit was lost")
	}
	if !strings.Contains(got.Merged, "rewritten by the layer") {
		t.Error("the layer's change was lost")
	}
}

func TestEmptyBaseWithBothSidesAddingConflicts(t *testing.T) {
	got := Three("", lines("user"), lines("layer"))
	if got.Clean() {
		t.Fatalf("two different files from nothing is a conflict, got %q", got.Merged)
	}
}

func TestEmptyInputsAreHandled(t *testing.T) {
	if got := Three("", "", ""); !got.Clean() || got.Merged != "" {
		t.Fatalf("got %q clean=%v", got.Merged, got.Clean())
	}
	// A file the layer emptied, untouched locally.
	if got := Three(lines("a"), lines("a"), ""); !got.Clean() || got.Merged != "" {
		t.Fatalf("got %q clean=%v", got.Merged, got.Clean())
	}
	// A file the user emptied, untouched by the layer.
	if got := Three(lines("a"), "", lines("a")); !got.Clean() || got.Merged != "" {
		t.Fatalf("got %q clean=%v", got.Merged, got.Clean())
	}
}

func TestTrailingNewlineIsPreserved(t *testing.T) {
	base := "a\nb\n"
	local := "a\nB\n"
	incoming := "a\nb\nc\n"

	got := Three(base, local, incoming)
	if !got.Clean() {
		t.Fatalf("expected clean, got %+v", got.Conflicts)
	}
	if !strings.HasSuffix(got.Merged, "\n") {
		t.Errorf("trailing newline lost: %q", got.Merged)
	}
	if strings.HasSuffix(got.Merged, "\n\n") {
		t.Errorf("trailing newline duplicated: %q", got.Merged)
	}
}

func TestNoChangeAnywhereIsIdentity(t *testing.T) {
	base := lines("a", "b", "c")
	got := Three(base, base, base)
	if !got.Clean() || got.Merged != base {
		t.Fatalf("got %q", got.Merged)
	}
}

func TestMergeIsSymmetricInWhatItAccepts(t *testing.T) {
	// Swapping which side is "local" must not change whether a merge is clean.
	base := lines("one", "two", "three", "four")
	a := lines("ONE", "two", "three", "four")
	b := lines("one", "two", "three", "FOUR")

	first := Three(base, a, b)
	second := Three(base, b, a)
	if first.Clean() != second.Clean() {
		t.Fatalf("cleanliness depends on argument order: %v vs %v", first.Clean(), second.Clean())
	}
	if first.Merged != second.Merged {
		t.Fatalf("merge is not order-independent:\n%q\n%q", first.Merged, second.Merged)
	}
}

func TestMarkersRenderBothSides(t *testing.T) {
	base := lines("a", "b", "c")
	local := lines("a", "USER", "c")
	incoming := lines("a", "LAYER", "c")

	got := WithMarkers(base, local, incoming, "yours", "ilk/record 0.2.0")
	for _, want := range []string{"<<<<<<< yours", "USER", "=======", "LAYER", ">>>>>>> ilk/record 0.2.0"} {
		if !strings.Contains(got, want) {
			t.Errorf("marker output missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, ConflictMarker) {
		t.Error("output should be findable by the marker constant the check scans for")
	}
}

func TestMarkersAreOnlyWrittenForRealConflicts(t *testing.T) {
	base := lines("a", "b", "c")
	local := lines("a", "USER", "c")
	incoming := lines("a", "b", "c")

	got := WithMarkers(base, local, incoming, "yours", "theirs")
	if strings.Contains(got, ConflictMarker) {
		t.Fatalf("a clean region must not be marked up:\n%s", got)
	}
	if got != local {
		t.Fatalf("got %q want %q", got, local)
	}
}

func TestOversizedInputIsDeclinedNotGuessed(t *testing.T) {
	// Two large files with no common prefix or suffix exceed the alignment bound.
	// Declining is correct; silently returning one side would lose data.
	var a, b []string
	for i := 0; i < 3000; i++ {
		a = append(a, fmt.Sprintf("a%d", i))
		b = append(b, fmt.Sprintf("b%d", i))
	}
	got := Three(lines(a...), lines(b...), lines(a...))
	if !got.Declined {
		t.Skip("alignment bound not reached at this size; nothing to assert")
	}
	if got.Clean() {
		t.Fatal("a declined merge must not report itself as clean")
	}
}

func TestSummariseNamesTheLines(t *testing.T) {
	base := lines("a", "b", "c", "d", "e")
	local := lines("A", "b", "c", "d", "E")
	incoming := lines("A!", "b", "c", "d", "E!")

	got := Three(base, local, incoming)
	if got.Clean() {
		t.Fatal("expected conflicts")
	}
	summary := got.Summarise()
	if !strings.Contains(summary, "2 hunks") {
		t.Errorf("summary should count the hunks: %q", summary)
	}
}
