package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/registry"
)

func TestSearchForFiltersByTraitAndShowsIt(t *testing.T) {
	root := emptyRepo(t)
	code, stdout, stderr := invoke(t, "-C", root, "--json", "search", "--for", "ui")
	if code != exitOK {
		t.Fatalf("search --for ui: exit %d\n%s", code, stderr)
	}
	var rows []struct {
		ID  string   `json:"id"`
		For []string `json:"for"`
	}
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.ID)
		hasUI := false
		for _, f := range r.For {
			hasUI = hasUI || f == "ui"
		}
		if !hasUI {
			t.Errorf("%s is for %v, which leaves out ui", r.ID, r.For)
		}
	}
	sort.Strings(ids)
	if want := []string{"ilk/html-wireframe", "ilk/preview", "ilk/visual-qa"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("--for ui lists %v, want %v — and not every layer that is for any repository", ids, want)
	}

	code, stdout, _ = invoke(t, "-C", root, "search", "--for", "ui")
	if code != exitOK || !strings.Contains(stdout, "for ui") || !strings.Contains(stdout, "@ui") {
		t.Errorf("text search should show the trait and the bundle made of those layers:\n%s", stdout)
	}

	code, _, stderr = invoke(t, "-C", root, "--json", "search", "--for", "frontend")
	if code != exitUsage {
		t.Fatalf("an unknown trait: exit %d, want %d", code, exitUsage)
	}
	if c, fix := errorOf(t, stderr); c != "usage" || !strings.Contains(fix, "unattended-agents") {
		t.Errorf("an unknown trait should list the real ones: %q %q", c, fix)
	}
}

func TestInfoShowsWhoALayerIsFor(t *testing.T) {
	root := emptyRepo(t)
	code, stdout, _ := invoke(t, "-C", root, "--json", "info", "gates")
	if code != exitOK {
		t.Fatalf("info: exit %d", code)
	}
	var info struct {
		For []string `json:"for"`
	}
	if err := json.Unmarshal([]byte(stdout), &info); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(info.For, []string{"any"}) {
		t.Fatalf("gates is for %v, want [any]", info.For)
	}
	if code, stdout, _ := invoke(t, "-C", root, "info", "gates"); code != exitOK || !strings.Contains(stdout, "for: any") {
		t.Errorf("text info should say who the layer is for:\n%s", stdout)
	}
}

func TestInitSuggestsLayersForWhatItFindsAndAddsNone(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"private": true, "dependencies": {"react": "19", "expo": "52"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := invoke(t, "-C", root, "init", "--yes")
	if code != exitOK {
		t.Fatalf("init: exit %d\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "package.json depends on react") || !strings.Contains(stdout, "ilk add @preview @ui") {
		t.Errorf("init should name what it found and the bundle for it:\n%s", stdout)
	}
	for _, id := range layersIn(t, root) {
		if id == "ilk/html-wireframe" || id == "ilk/visual-qa" || id == "ilk/preview" {
			t.Fatalf("a suggestion was added without being asked for: %v", layersIn(t, root))
		}
	}
}

func TestInitJSONKeepsSuggestionsOffStdout(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "infra", "dns"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "infra", "dns", "main.tf"), []byte("terraform {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := invoke(t, "-C", root, "--json", "init")
	if code != exitOK {
		t.Fatalf("init --json: exit %d\n%s", code, stderr)
	}
	var plan any
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("stdout must stay one JSON document: %v\n%s", err, stdout)
	}
	if !strings.Contains(stderr, "ilk add infra") {
		t.Errorf("the suggestion belongs on stderr under --json:\n%s", stderr)
	}
}

func TestCoverWithBundlesUsesOnlyBundlesOfOfferedLayers(t *testing.T) {
	// In id order, as registry.Bundles gives them.
	bundles := []registry.Bundle{
		{ID: "preview", Layers: []string{"ilk/preview", "ilk/visual-qa"}},
		{ID: "ui", Layers: []string{"ilk/html-wireframe", "ilk/visual-qa"}},
	}
	cases := []struct {
		ids  []string
		want []string
	}{
		{[]string{"ilk/html-wireframe", "ilk/preview", "ilk/visual-qa"}, []string{"@preview", "@ui"}},
		{[]string{"ilk/html-wireframe", "ilk/visual-qa"}, []string{"@ui"}},
		// @preview would add visual-qa, which is not on offer, so it is not used.
		{[]string{"ilk/deprecation", "ilk/preview"}, []string{"deprecation", "preview"}},
	}
	for _, c := range cases {
		var names []string
		for _, id := range c.ids {
			names = append(names, strings.TrimPrefix(id, "ilk/"))
		}
		if got := coverWithBundles(c.ids, names, bundles); !reflect.DeepEqual(got, c.want) {
			t.Errorf("cover %v = %v, want %v", c.ids, got, c.want)
		}
	}
}

func TestInitSuggestsEachLayerOnceAcrossTraits(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"package.json":   `{"private": true, "dependencies": {"react": "19", "expo": "52"}}`,
		"wrangler.jsonc": `{"name": "app", "main": "worker/index.ts"}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := suggestFor(root, map[string]bool{})
	seen := map[string]string{}
	for _, s := range got {
		for _, id := range s.Layers {
			if first, ok := seen[id]; ok {
				t.Errorf("%s suggested for %s and again for %s", id, first, s.Trait)
			}
			seen[id] = s.Trait
		}
	}
	if seen["ilk/preview"] == "" {
		t.Errorf("a React app on a Worker should be offered preview: %+v", got)
	}
}
