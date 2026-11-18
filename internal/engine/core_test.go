package engine

import (
	"os"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/builtin"
	"github.com/coflounder/ilk/internal/config"
)

// The rest of this package's tests exercise the mechanics of adding and
// removing layers, record among them, and assert that removal restores the
// repository. With the core tier implied, record could never be removed, so
// those tests run with the tier off and the tests below switch it on.
func TestMain(m *testing.M) {
	coreTier = nil
	os.Exit(m.Run())
}

func withCoreTier(t *testing.T) {
	t.Helper()
	coreTier = builtin.Core
	t.Cleanup(func() { coreTier = nil })
}

func TestTheCoreTierIsImpliedWhenTheConfigOmitsIt(t *testing.T) {
	withCoreTier(t)
	root := fixture(t)
	configWith(t, root, "ilk/gates")
	apply(t, root)

	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, l := range p.Layers {
		ids = append(ids, l.ID())
		if l.Core() != builtin.IsCore(l.ID()) {
			t.Errorf("%s: Core() disagrees with the tier", l.ID())
		}
		if l.Implied != l.Core() {
			t.Errorf("%s: implied=%v, but the config lists only gates", l.ID(), l.Implied)
		}
	}
	if got := strings.Join(ids, ","); got != "ilk/gates,ilk/record,ilk/toolkit" {
		t.Fatalf("layers in id order with the core tier slotted in, got %s", got)
	}
	if !exists(root, "docs/reference/README.md") {
		t.Fatal("record's directories should be on disk though the config never named it")
	}

	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Layers) != 1 {
		t.Fatalf("an implied layer must not be written into the config: %+v", cfg.Layers)
	}
}

func TestAListedCoreLayerKeepsItsVariables(t *testing.T) {
	withCoreTier(t)
	root := fixture(t)
	cfg := configWith(t, root)
	cfg.Set(config.LayerRef{ID: "ilk/record", Vars: map[string]string{"reference_dir": "current"}})
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	apply(t, root)

	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	l, ok := p.Layer("record")
	if !ok || l.Implied {
		t.Fatalf("a listed core layer is not implied: %+v", l)
	}
	if !exists(root, "docs/current/README.md") {
		t.Fatal("the listed entry's variables should apply")
	}
	if _, ok := p.Layer("toolkit"); !ok {
		t.Fatal("the unlisted core layer is still implied")
	}
}

func TestBareLeavesTheCoreTierOut(t *testing.T) {
	withCoreTier(t)
	root := fixture(t)
	configWith(t, root, "ilk/gates")
	p, err := LoadWith(root, "test", Options{Bare: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Layers) != 1 || p.Layers[0].ID() != "ilk/gates" {
		t.Fatalf("bare resolves only what the config lists, got %d layers", len(p.Layers))
	}
}
