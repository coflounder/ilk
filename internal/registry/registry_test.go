package registry

import (
	"strings"
	"testing"
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
