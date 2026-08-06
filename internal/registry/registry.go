// Package registry is layer discovery.
//
// The whole registry is a list of names and where to fetch them, embedded in the
// binary so `ilk search` works with no network. There is no server and no
// publishing step beyond pushing a git repository and adding an entry — which is
// the point: publishing a layer should cost about as much as publishing the post
// that described the idea.
//
// The embedded copy is a snapshot taken at release time. A layer added to the
// index afterwards is still adoptable by its source; it just will not be listed,
// and `ilk search` says so rather than implying the list is exhaustive.
package registry

import (
	_ "embed"
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
	Requires []string `yaml:"requires,omitempty" json:"requires,omitempty"`
}

// Builtin reports whether the layer ships inside the binary.
func (e Entry) Builtin() bool { return e.Source == "builtin" }

// Ref is what a user would pass to `ilk adopt`.
func (e Entry) Ref() string {
	if e.Builtin() {
		return e.Name()
	}
	return e.Source
}

// Name is the short name, the last segment of the id.
func (e Entry) Name() string {
	if _, after, ok := strings.Cut(e.ID, "/"); ok {
		return after
	}
	return e.ID
}

type index struct {
	Version int     `yaml:"version"`
	Layers  []Entry `yaml:"layers"`
}

// All returns every listed layer, sorted so built-ins come first — those are the
// ones adoptable with no network, which is what somebody with a fresh install
// wants to see at the top.
func All() ([]Entry, error) {
	var idx index
	if err := yaml.Unmarshal(indexData, &idx); err != nil {
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
			e.ID, e.Summary, e.Arc, e.Kind, strings.Join(e.Requires, " "),
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
