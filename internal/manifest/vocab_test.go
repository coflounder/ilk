package manifest

import (
	"strings"
	"testing"
)

func TestForAndFacetsAreClosedLists(t *testing.T) {
	const base = "id: test/x\nversion: 0.1.0\nsummary: Vocabulary fixture.\n"
	for _, tc := range []struct {
		name, body, wantErr string
	}{
		{"known traits", "for: [ui, library]\n", ""},
		{"any alone", "for: [any]\n", ""},
		{"known facets", "facets:\n  arc: quality\n  kind: gate\n", ""},
		{"unknown trait", "for: [frontend]\n", `"frontend" is not a repository trait`},
		{"any with others", "for: [any, ui]\n", "stands alone"},
		{"a trait twice", "for: [ui, ui]\n", "listed twice"},
		{"unknown arc", "facets:\n  arc: shipping\n", `arc "shipping" is not one ilk knows`},
		{"unknown kind", "facets:\n  kind: target\n", `kind "target" is not one ilk knows`},
		{"unknown facet", "facets:\n  tier: core\n", `"tier" is not a facet`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(base+tc.body), "test")
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("should parse: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("want an error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}
