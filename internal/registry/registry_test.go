package registry

import (
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/builtin"
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
