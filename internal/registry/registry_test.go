package registry

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/builtin"
	"github.com/coflounder/ilk/internal/manifest"
)

func TestEveryBundleMemberIsListed(t *testing.T) {
	bundles, err := Bundles()
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) == 0 {
		t.Fatal("the index should ship bundles")
	}
	for _, b := range bundles {
		refs, err := Expand(b.Ref())
		if err != nil {
			t.Errorf("%s: %v", b.Ref(), err)
		}
		if len(refs) != len(b.Layers) || len(refs) < 2 {
			t.Errorf("%s: a bundle needs at least two members, all listed: %v", b.Ref(), refs)
		}
	}
}

func TestAnUnknownBundleNamesTheRealOnes(t *testing.T) {
	_, err := Expand("@nope")
	if err == nil || !strings.Contains(err.Error(), "@plans") {
		t.Fatalf("%v", err)
	}
}

func TestTheIndexListsEveryBuiltinButTheCoreTier(t *testing.T) {
	entries, err := All()
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]Entry{}
	for _, e := range entries {
		listed[e.ID] = e
	}
	for _, id := range builtin.Core {
		if _, ok := listed[id]; ok {
			t.Errorf("%s is core: every repository has it, so the index must not offer it", id)
		}
	}
	for _, id := range builtin.Default {
		e, ok := listed[id]
		if !ok || !e.Builtin() {
			t.Errorf("%s is in the default tier, which init adds offline: it must be listed as builtin, got %+v", id, e)
		}
	}
}

// The index is what `ilk search` reads, offline, and the manifest is what
// `ilk info` reads. For every layer shipped from this repository the two must
// say the same thing, or search and info describe two different layers.
func TestTheIndexAgreesWithEveryShippedManifest(t *testing.T) {
	entries, err := All()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		var dir string
		switch {
		case e.Builtin():
			dir = filepath.Join("..", "builtin", "layers", e.Name())
		case strings.HasPrefix(e.Source, "gh:coflounder/ilk/"):
			dir = filepath.Join("..", "..", strings.TrimPrefix(e.Source, "gh:coflounder/ilk/"))
		default:
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "layer.yaml"))
		if err != nil {
			t.Errorf("%s: %v", e.ID, err)
			continue
		}
		m, err := manifest.Parse(data, dir)
		if err != nil {
			t.Errorf("%s: %v", e.ID, err)
			continue
		}
		if m.ID != e.ID {
			t.Errorf("%s: the manifest at %s is %s", e.ID, dir, m.ID)
		}
		if e.Arc != m.Facets["arc"] || e.Kind != m.Facets["kind"] {
			t.Errorf("%s: index says arc=%s kind=%s, manifest says %v", e.ID, e.Arc, e.Kind, m.Facets)
		}
		if !reflect.DeepEqual(e.For, m.For) {
			t.Errorf("%s: index says for %v, manifest says %v", e.ID, e.For, m.For)
		}
		if len(m.For) == 0 {
			t.Errorf("%s: every shipped layer says who it is for", e.ID)
		}
		want := append([]string(nil), m.Requires...)
		got := append([]string(nil), e.Requires...)
		sort.Strings(want)
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Errorf("%s: index says requires %v, manifest says %v", e.ID, got, want)
		}
	}
}

func TestEveryIndexedTraitAndFacetIsKnown(t *testing.T) {
	entries, err := All()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		for _, tr := range e.For {
			if !manifest.ValidTrait(tr) {
				t.Errorf("%s: for %q is not a trait", e.ID, tr)
			}
		}
	}
}
