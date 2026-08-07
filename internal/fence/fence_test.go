package fence

import (
	"strings"
	"testing"
)

var md = StyleFor("AGENTS.md")

func TestUpsertAppendsAndRoundTrips(t *testing.T) {
	original := "# My project\n\nSome prose a human wrote.\n"
	m := Marker{Layer: "ilk/record", Region: "contract"}

	got, err := Upsert(original, md, m, "The contract line.\n")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if !strings.Contains(got, "The contract line.") {
		t.Fatalf("body missing from %q", got)
	}
	if !strings.Contains(got, "Some prose a human wrote.") {
		t.Fatalf("human prose lost from %q", got)
	}

	back, removed, err := Remove(got, md, m)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !removed {
		t.Fatal("Remove reported nothing removed")
	}
	if back != original {
		t.Fatalf("adopt→drop is not lossless:\n got %q\nwant %q", back, original)
	}
}

func TestUpsertReplacesInPlaceAndPreservesNeighbours(t *testing.T) {
	m := Marker{Layer: "ilk/record", Region: "contract"}
	base := "# Title\n\nbefore\n"

	withRegion, err := Upsert(base, md, m, "v1\n")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	// A human adds prose after the region; it must survive an upgrade.
	withRegion += "\nafter — written by a human\n"

	upgraded, err := Upsert(withRegion, md, m, "v2\n")
	if err != nil {
		t.Fatalf("Upsert upgrade: %v", err)
	}
	if strings.Contains(upgraded, "v1") {
		t.Fatalf("old body survived upgrade: %q", upgraded)
	}
	if !strings.Contains(upgraded, "v2") {
		t.Fatalf("new body missing: %q", upgraded)
	}
	if !strings.Contains(upgraded, "after — written by a human") {
		t.Fatalf("human prose after the region was lost: %q", upgraded)
	}
	if !strings.Contains(upgraded, "before") {
		t.Fatalf("human prose before the region was lost: %q", upgraded)
	}
	if strings.Count(upgraded, "ilk:begin") != 1 {
		t.Fatalf("region duplicated: %q", upgraded)
	}
}

func TestRemoveKeepsSurroundingContentWhenRegionIsInterior(t *testing.T) {
	m := Marker{Layer: "l", Region: "r"}
	content := "head\n\n" + mustUpsert(t, "", md, m, "body\n") + "tail\n"

	got, removed, err := Remove(content, md, m)
	if err != nil || !removed {
		t.Fatalf("Remove: %v removed=%v", err, removed)
	}
	if got != "head\n\ntail\n" {
		t.Fatalf("got %q", got)
	}
}

func TestMultipleRegionsAreIndependent(t *testing.T) {
	a := Marker{Layer: "ilk/record", Region: "contract"}
	b := Marker{Layer: "ilk/gates", Region: "gates"}

	s := mustUpsert(t, "# Title\n", md, a, "A body\n")
	s = mustUpsert(t, s, md, b, "B body\n")

	if !Has(s, md, a) || !Has(s, md, b) {
		t.Fatal("expected both regions present")
	}

	s, _, err := Remove(s, md, a)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if Has(s, md, a) {
		t.Fatal("region a survived removal")
	}
	if !Has(s, md, b) {
		t.Fatalf("removing a destroyed b: %q", s)
	}
	if !strings.Contains(s, "B body") {
		t.Fatalf("b body lost: %q", s)
	}
}

func TestMarkersEnumeratesOrphans(t *testing.T) {
	a := Marker{Layer: "ilk/record", Region: "contract"}
	b := Marker{Layer: "third-party/thing", Region: "x"}
	s := mustUpsert(t, "", md, a, "one\n")
	s = mustUpsert(t, s, md, b, "two\n")

	got := Markers(s, md)
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("got %+v", got)
	}
}

func TestUnterminatedRegionIsAnActionableError(t *testing.T) {
	m := Marker{Layer: "l", Region: "r"}
	content := "<!-- ilk:begin layer=l region=r -->\nbody\n"

	_, err := Upsert(content, md, m, "new\n")
	if err == nil {
		t.Fatal("expected an error for an unterminated region")
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("error should name the line: %v", err)
	}
}

func TestAppendOnceIsIdempotent(t *testing.T) {
	m := Marker{Layer: "l", Region: "ignore"}
	base := "node_modules\n"

	once, err := AppendOnce(base, StyleFor(".gitignore"), m, "scratch/\n")
	if err != nil {
		t.Fatalf("AppendOnce: %v", err)
	}
	twice, err := AppendOnce(once, StyleFor(".gitignore"), m, "scratch/\n")
	if err != nil {
		t.Fatalf("AppendOnce twice: %v", err)
	}
	if once != twice {
		t.Fatalf("not idempotent:\n%q\n%q", once, twice)
	}
	if strings.Count(twice, "scratch/") != 1 {
		t.Fatalf("appended twice: %q", twice)
	}
}

func TestStyleForPicksCommentSyntax(t *testing.T) {
	cases := map[string]string{
		"AGENTS.md":             "<!--",
		".claude/settings.yaml": "#",
		"scripts/gate.sh":       "#",
		"main.go":               "//",
		".gitignore":            "#",
		"Makefile":              "#",
		"unknown.zzz":           "#",
	}
	for path, want := range cases {
		if got := StyleFor(path).Open; got != want {
			t.Errorf("StyleFor(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestExtractReturnsBody(t *testing.T) {
	m := Marker{Layer: "l", Region: "r"}
	s := mustUpsert(t, "top\n", md, m, "line one\nline two\n")

	body, ok, err := Extract(s, md, m)
	if err != nil || !ok {
		t.Fatalf("Extract: %v ok=%v", err, ok)
	}
	if body != "line one\nline two\n" {
		t.Fatalf("got %q", body)
	}
}

func TestMarkerLinesCarryAHumanWarning(t *testing.T) {
	m := Marker{Layer: "l", Region: "r"}
	s := mustUpsert(t, "", md, m, "x\n")
	if !strings.Contains(s, Warning) {
		t.Fatalf("begin marker should warn humans off: %q", s)
	}
}

func mustUpsert(t *testing.T, content string, style Style, m Marker, body string) string {
	t.Helper()
	got, err := Upsert(content, style, m, body)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	return got
}
