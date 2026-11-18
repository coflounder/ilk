// Package registry is layer discovery.
//
// The whole registry is a list of names and where to fetch them, embedded in the
// binary so `ilk search` works with no network. There is no server and no
// publishing step beyond pushing a git repository and adding an entry — which is
// the point: publishing a layer should cost about as much as publishing the post
// that described the idea.
//
// The embedded copy is a snapshot taken at release time. A layer added to the
// index afterwards can still be added by its source; it just will not be listed,
// and `ilk search` says so rather than implying the list is exhaustive.
package registry

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed registry.yaml
var indexData []byte

// Entry is one listed layer.
type Entry struct {
	ID       string   `yaml:"id" json:"id"`
	Source   string   `yaml:"source" json:"source"`
	Summary  string   `yaml:"summary" json:"summary"`
	Arc      string   `yaml:"arc,omitempty" json:"arc,omitempty"`
	Kind     string   `yaml:"kind,omitempty" json:"kind,omitempty"`
	For      []string `yaml:"for,omitempty" json:"for,omitempty"`
	Requires []string `yaml:"requires,omitempty" json:"requires,omitempty"`
}

// IsFor reports whether the entry declares a trait. `any` matches only `any`:
// asking for the layers meant for interfaces should not list every layer
// meant for every repository.
func (e Entry) IsFor(trait string) bool {
	for _, t := range e.For {
		if t == trait {
			return true
		}
	}
	return false
}

// Builtin reports whether the layer ships inside the binary.
func (e Entry) Builtin() bool { return e.Source == "builtin" }

// Ref is what a user would pass to `ilk add`.
func (e Entry) Ref() string {
	if e.Builtin() {
		return e.Name()
	}
	return e.Source
}

// Lookup finds a layer in the index by its id (ilk/blueprint) or its short name
// (blueprint). Only ilk's own index is searched: a short name never resolves to
// a layer somebody else published.
func Lookup(name string) (Entry, bool) {
	idx, err := load()
	if err != nil {
		return Entry{}, false
	}
	for _, e := range idx.Layers {
		if e.ID == name || e.Name() == name {
			return e, true
		}
	}
	return Entry{}, false
}

// Name is the short name, the last segment of the id.
func (e Entry) Name() string {
	if _, after, ok := strings.Cut(e.ID, "/"); ok {
		return after
	}
	return e.ID
}

type index struct {
	Version int      `yaml:"version"`
	Layers  []Entry  `yaml:"layers"`
	Bundles []Bundle `yaml:"bundles"`
}

// BundlePrefix marks a bundle wherever a layer reference is accepted.
const BundlePrefix = "@"

// Bundle is a named set of layers usually adopted together. It contributes
// nothing itself: adding one adds each member, and each member stays removable
// on its own.
type Bundle struct {
	ID      string   `yaml:"id" json:"id"`
	Summary string   `yaml:"summary" json:"summary"`
	Layers  []string `yaml:"layers" json:"layers"`
}

// Ref is what a user would pass to `ilk add`.
func (b Bundle) Ref() string { return BundlePrefix + b.ID }

func load() (index, error) {
	var idx index
	err := yaml.Unmarshal(indexData, &idx)
	return idx, err
}

// IsBundleRef reports whether a reference names a bundle.
func IsBundleRef(ref string) bool { return strings.HasPrefix(ref, BundlePrefix) }

// Bundles lists every bundle, sorted by id.
func Bundles() ([]Bundle, error) {
	idx, err := load()
	if err != nil {
		return nil, err
	}
	sort.Slice(idx.Bundles, func(i, j int) bool { return idx.Bundles[i].ID < idx.Bundles[j].ID })
	return idx.Bundles, nil
}

// Expand resolves `@name` to the references of its members, in the bundle's
// order, so they can be added like any other layer.
func Expand(ref string) ([]string, error) {
	name := strings.TrimPrefix(ref, BundlePrefix)
	idx, err := load()
	if err != nil {
		return nil, err
	}
	byID := map[string]Entry{}
	for _, e := range idx.Layers {
		byID[e.ID] = e
	}
	var known []string
	for _, b := range idx.Bundles {
		known = append(known, b.Ref())
		if b.ID != name {
			continue
		}
		refs := make([]string, 0, len(b.Layers))
		for _, id := range b.Layers {
			e, ok := byID[id]
			if !ok {
				return nil, fmt.Errorf("bundle %s lists %s, which is not in the index", b.Ref(), id)
			}
			refs = append(refs, e.Ref())
		}
		return refs, nil
	}
	sort.Strings(known)
	return nil, fmt.Errorf("no bundle called %s — the bundles are %s", ref, strings.Join(known, ", "))
}

// All returns every listed layer, sorted so built-ins come first — those are the
// ones addable with no network, which is what somebody with a fresh install
// wants to see at the top.
func All() ([]Entry, error) {
	idx, err := load()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(idx.Layers, func(i, j int) bool {
		a, b := idx.Layers[i], idx.Layers[j]
		if a.Builtin() != b.Builtin() {
			return a.Builtin()
		}
		return a.ID < b.ID
	})
	return idx.Layers, nil
}

// Search returns entries matching every term, case-insensitively, across the id,
// summary and facets. An empty query matches everything.
func Search(query string) ([]Entry, error) {
	all, err := All()
	if err != nil {
		return nil, err
	}
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return all, nil
	}

	var out []Entry
	for _, e := range all {
		haystack := strings.ToLower(strings.Join([]string{
			e.ID, e.Summary, e.Arc, e.Kind, strings.Join(e.For, " "), strings.Join(e.Requires, " "),
		}, " "))
		matched := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				matched = false
				break
			}
		}
		if matched {
			out = append(out, e)
		}
	}
	return out, nil
}
