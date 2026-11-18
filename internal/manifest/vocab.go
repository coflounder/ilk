package manifest

import "strings"

// The words a manifest uses to say who a layer is for. Each list is closed:
// free text drifts — one author's "frontend" is another's "web" — and a word
// nobody else uses finds nobody. A value outside a list is a manifest error, so
// adding a word is a change to ilk, made once, for everyone.

// Trait is a property of a repository that a layer is meant for. `for:` lists
// them, `ilk search --for` filters by them, and `ilk init` detects the simple
// ones and suggests the layers that declare them.
type Trait struct {
	Name    string
	Meaning string
}

// Traits are the repository traits `for:` may name, in display order.
var Traits = []Trait{
	{"any", "every repository, whatever it builds"},
	{"ui", "has a user interface people look at"},
	{"service", "runs somewhere and serves requests"},
	{"library", "is consumed by other code through its API"},
	{"infra", "declares infrastructure as code"},
	{"unattended-agents", "has agents working with nobody watching"},
	{"multi-agent", "has several agents or harnesses working at once"},
}

// TraitAny is the trait of a layer meant for every repository. It stands
// alone: a layer for any repository is not also for some of them.
const TraitAny = "any"

// ValidTrait reports whether a name is one of the Traits.
func ValidTrait(name string) bool {
	for _, t := range Traits {
		if t.Name == name {
			return true
		}
	}
	return false
}

// TraitNames lists the traits in display order.
func TraitNames() []string {
	names := make([]string, 0, len(Traits))
	for _, t := range Traits {
		names = append(names, t.Name)
	}
	return names
}

// Facets are the discovery tags every manifest already carried. The values are
// the ones the layers shipped with ilk use; anything else is a manifest error.
var Facets = map[string][]string{
	// arc: where in a piece of work the layer acts.
	"arc": {"context", "planning", "execution", "quality", "release", "operations"},
	// kind: what sort of thing the layer adds.
	"kind": {"record", "process", "gate", "harness", "integration"},
}

// validateFor checks a `for:` list.
func validateFor(traits []string, bad func(string, ...any)) {
	seen := map[string]bool{}
	for _, t := range traits {
		if !ValidTrait(t) {
			bad("for: %q is not a repository trait ilk knows — use one of %s", t, strings.Join(TraitNames(), ", "))
			continue
		}
		if seen[t] {
			bad("for: %q is listed twice", t)
		}
		seen[t] = true
	}
	if seen[TraitAny] && len(traits) > 1 {
		bad("for: %q stands alone — a layer for any repository is not also for some of them; drop the others or drop %q", TraitAny, TraitAny)
	}
}

// validateFacets checks `facets:` against the closed lists.
func validateFacets(facets map[string]string, bad func(string, ...any)) {
	for key, value := range facets {
		allowed, known := Facets[key]
		if !known {
			bad("facets: %q is not a facet — the facets are arc and kind", key)
			continue
		}
		ok := false
		for _, a := range allowed {
			if a == value {
				ok = true
			}
		}
		if !ok {
			bad("facets: %s %q is not one ilk knows — use one of %s", key, value, strings.Join(allowed, ", "))
		}
	}
}
