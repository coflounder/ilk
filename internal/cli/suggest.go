package cli

import (
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/detect"
	"github.com/coflounder/ilk/internal/registry"
)

// suggestion is what init offers for one detected trait: the evidence, and the
// layers declared for that trait which the repository does not have yet.
type suggestion struct {
	detect.Finding
	// Add is what to pass to `ilk add`: one bundle when a bundle is exactly
	// the layers on offer, otherwise each layer by name.
	Add    []string `json:"add"`
	Layers []string `json:"layers"`
}

// suggestFor turns detected traits into layers to suggest, leaving out what the
// repository already has. A trait with nothing left to suggest is left out.
func suggestFor(root string, have map[string]bool) []suggestion {
	entries, err := registry.All()
	if err != nil {
		return nil
	}
	bundles, _ := registry.Bundles()

	var out []suggestion
	for _, f := range detect.Traits(root) {
		var ids, names []string
		for _, e := range entries {
			if e.IsFor(f.Trait) && !have[e.ID] {
				ids = append(ids, e.ID)
				names = append(names, e.Name())
			}
		}
		if len(ids) == 0 {
			continue
		}
		s := suggestion{Finding: f, Layers: ids, Add: names}
		for _, b := range bundles {
			if sameSet(b.Layers, ids) {
				s.Add = []string{b.Ref()}
			}
		}
		out = append(out, s)
	}
	return out
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// printSuggestions writes the suggestions with w: stdout normally, stderr under
// --json so the plan on stdout stays the only thing there.
func printSuggestions(w func(string, ...any), list []suggestion) {
	if len(list) == 0 {
		return
	}
	w("\n%s\n", sty.bold("Suggested for what this repository looks like"))
	for _, s := range list {
		w("  %-10s %s\n", s.Trait, sty.dim(s.Evidence))
		w("  %-10s ilk add %s\n", "", strings.Join(s.Add, " "))
	}
	w("  %s\n", sty.dim("Nothing was added. `ilk info <layer>` shows what each would write."))
}
