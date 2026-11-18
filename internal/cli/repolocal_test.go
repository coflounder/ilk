package cli

import (
	"strings"
	"testing"
)

// maintainer is ilk's own: it lives in .ilk/layers/maintainer and is
// referenced by path from ilk's config, so search has nothing to offer anyone
// else under that name.
func TestSearchDoesNotListMaintainer(t *testing.T) {
	root := emptyRepo(t)
	for _, args := range [][]string{
		{"--json", "search", "--all"},
		{"--json", "search", "maintainer"},
		{"--json", "search", "--for", "library"},
	} {
		code, stdout, stderr := invoke(t, append([]string{"-C", root}, args...)...)
		if code != exitOK {
			t.Fatalf("%v: exit %d\n%s", args, code, stderr)
		}
		if strings.Contains(stdout, "ilk/maintainer") {
			t.Errorf("%v lists ilk/maintainer:\n%s", args, stdout)
		}
	}
}
