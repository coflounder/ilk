package cli

import (
	"strings"

	"github.com/coflounder/ilk/internal/detect"
	"github.com/coflounder/ilk/internal/registry"
)

// suggestion is what init offers for one detected trait: the evidence, and the
// layers declared for that trait which the repository does not have yet.
type suggestion struct {
	detect.Finding
	// Add is what to pass to `ilk add`: the bundles made only of layers on
	// offer, then any layer no such bundle covers, by name.
	Add    []string `json:"add"`
	Layers []string `json:"layers"`
}

// suggestFor turns detected traits into layers to suggest, leaving out what the
// repository already has and what an earlier trait already suggested. A trait
// with nothing left to suggest is left out.
func suggestFor(root string, have map[string]bool) []suggestion {
	entries, err := registry.All()
	if err != nil {
		return nil
	}
	bundles, _ := registry.Bundles()

	var out []suggestion
	offered := map[string]bool{}
	for _, f := range detect.Traits(root) {
		var ids, names []string
		for _, e := range entries {
			if e.IsFor(f.Trait) && !have[e.ID] && !offered[e.ID] {
				offered[e.ID] = true
				ids = append(ids, e.ID)
				names = append(names, e.Name())
			}
		}
		if len(ids) == 0 {
			continue
		}
		add := coverWithBundles(ids, names, bundles)
		// The infra layer defaults to Pulumi and seeds a Pulumi.yaml, so on a
		// Terraform or OpenTofu estate the suggestion carries the tool with it.
		// --var takes one layer, so it rides along only when infra is alone.
		if f.Trait == "infra" && len(add) == 1 && add[0] == "infra" {
			if tool := detect.InfraTool(root); tool == "terraform" || tool == "opentofu" {
				add = append(add, "--var", "tool="+tool)
			}
		}
		out = append(out, suggestion{Finding: f, Layers: ids, Add: add})
	}
	return out
}

// coverWithBundles says how to add the layers on offer in as few words as it
// can: bundles made only of offered layers, each one chosen for covering the
// most layers not yet covered (the first by name wins a tie), then the layers
// no bundle covered, by name. A bundle that would add a layer not on offer is
// never used.
func coverWithBundles(ids, names []string, bundles []registry.Bundle) []string {
	offered := map[string]bool{}
	for _, id := range ids {
		offered[id] = true
	}
	var usable []registry.Bundle
	for _, b := range bundles {
		ok := len(b.Layers) > 0
		for _, l := range b.Layers {
			ok = ok && offered[l]
		}
		if ok {
			usable = append(usable, b)
		}
	}

	covered := map[string]bool{}
	var add []string
	for {
		best, gain := -1, 0
		for i, b := range usable {
			n := 0
			for _, l := range b.Layers {
				if !covered[l] {
					n++
				}
			}
			if n > gain {
				best, gain = i, n
			}
		}
		if best < 0 {
			break
		}
		add = append(add, usable[best].Ref())
		for _, l := range usable[best].Layers {
			covered[l] = true
		}
	}
	for i, id := range ids {
		if !covered[id] {
			add = append(add, names[i])
		}
	}
	return add
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
