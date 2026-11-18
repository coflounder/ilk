package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/coflounder/ilk/internal/checks"
	"github.com/coflounder/ilk/internal/registry"
)

func TestPlanExitCodeSaysWhetherAnythingIsPending(t *testing.T) {
	root := contractRepo(t)
	if code, _, _ := invoke(t, "-C", root, "plan"); code != exitOK {
		t.Fatalf("plan without --exit-code: exit %d, want 0", code)
	}
	if code, _, stderr := invoke(t, "-C", root, "plan", "--exit-code"); code != exitPending {
		t.Fatalf("plan --exit-code with changes pending: exit %d, want %d\n%s", code, exitPending, stderr)
	}
	if code, _, _ := invoke(t, "-C", root, "status", "--exit-code"); code != exitPending {
		t.Fatalf("status --exit-code with drift: exit %d, want %d", code, exitPending)
	}
	if code, _, stderr := invoke(t, "-C", root, "apply", "--yes"); code != exitOK {
		t.Fatalf("apply: exit %d\n%s", code, stderr)
	}
	if code, _, _ := invoke(t, "-C", root, "plan", "--exit-code"); code != exitOK {
		t.Fatalf("plan --exit-code in sync: exit %d, want 0", code)
	}
	if code, _, _ := invoke(t, "-C", root, "status", "--exit-code"); code != exitOK {
		t.Fatalf("status --exit-code in sync: exit %d, want 0", code)
	}
}

func TestDurationIsReportedInMilliseconds(t *testing.T) {
	b, err := json.Marshal(checks.Result{ID: "x", Duration: 1500 * time.Microsecond})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(b, &got)
	if got["duration_ms"] != 1.5 || got["id"] != "x" {
		t.Fatalf("got %s", b)
	}
}

func TestAnIndexedLayerResolvesByShortNameAndId(t *testing.T) {
	all, err := registry.All()
	if err != nil {
		t.Fatal(err)
	}
	var published registry.Entry
	for _, e := range all {
		if !e.Builtin() {
			published = e
			break
		}
	}
	for _, name := range []string{published.Name(), published.ID} {
		e, ok := registry.Lookup(name)
		if !ok || e.Source != published.Source {
			t.Errorf("Lookup(%q) = %v, %v", name, e, ok)
		}
	}
	if _, ok := registry.Lookup("no-such-layer"); ok {
		t.Error("an unknown name resolved")
	}
	for _, name := range []string{"./blueprint", "gh:x/y", "a/b/c", "blueprint@v1"} {
		if isBareName(name) {
			t.Errorf("%q should not be looked up in the index", name)
		}
	}
}

func TestAnUnknownLayerIsAUsageError(t *testing.T) {
	root := contractRepo(t)
	code, _, stderr := invoke(t, "-C", root, "info", "no-such-layer")
	if code != exitUsage || !strings.Contains(stderr, "ilk search") {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
}
