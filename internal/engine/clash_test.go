package engine

import (
	"errors"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/manifest"
)

func layerWith(id string, edit func(*manifest.Layer)) *manifest.Layer {
	m := &manifest.Layer{ID: id, Version: "0.1.0", Summary: "x"}
	if edit != nil {
		edit(m)
	}
	return m
}

func TestEachJobHasOneLayer(t *testing.T) {
	for _, tc := range []struct {
		kind string
		edit func(*manifest.Layer)
	}{
		{"capability", func(m *manifest.Layer) { m.Provides = manifest.Provides{"role.memory": ""} }},
		{"skill", func(m *manifest.Layer) { m.Skills = []manifest.Skill{{Name: "recall"}} }},
		{"check", func(m *manifest.Layer) { m.Checks = []manifest.Check{{ID: "memory.fresh"}} }},
		{"mcp", func(m *manifest.Layer) { m.MCP = []manifest.MCPServer{{Name: "memory"}} }},
		{"hook", func(m *manifest.Layer) { m.Hooks = []manifest.Hook{{Event: "turn-end", Name: "save"}} }},
	} {
		got := FindClashes([]*manifest.Layer{layerWith("acme/claude-mem", tc.edit), layerWith("acme/llm-wiki", tc.edit)})
		if len(got) != 1 || got[0].Kind != tc.kind {
			t.Errorf("%s: got %+v, want one %s clash", tc.kind, got, tc.kind)
			continue
		}
		if got[0].Layers[0] != "acme/claude-mem" || got[0].Layers[1] != "acme/llm-wiki" {
			t.Errorf("%s: the clash should name both layers in order: %v", tc.kind, got[0].Layers)
		}
	}
}

func TestTwoLayersWithOneShortNameClash(t *testing.T) {
	got := FindClashes([]*manifest.Layer{layerWith("acme/memory", nil), layerWith("other/memory", nil)})
	if len(got) != 1 || got[0].Kind != "name" || !strings.Contains(got[0].String(), "ilk memory") {
		t.Fatalf("got %+v", got)
	}
}

func TestWhatStacksDoesNotClash(t *testing.T) {
	// Several layers checking a commit, and blocking turn-end hooks that each
	// add a condition for being done, are the point of hooks, not a clash.
	hooks := func(name string) func(*manifest.Layer) {
		return func(m *manifest.Layer) {
			m.Hooks = []manifest.Hook{
				{Event: "pre-commit", Name: name + "-scan"},
				{Event: "turn-end", Name: name + "-done", Blocking: true},
			}
			m.Instructions = []manifest.Instruction{{ID: "main", Inline: "x"}}
		}
	}
	if got := FindClashes([]*manifest.Layer{layerWith("acme/one", hooks("one")), layerWith("acme/two", hooks("two"))}); len(got) != 0 {
		t.Fatalf("hooks on a shared event and AGENTS.md instructions should stack: %+v", got)
	}
	// A layer never clashes with itself.
	self := layerWith("acme/one", func(m *manifest.Layer) {
		m.Hooks = []manifest.Hook{{Event: "pre-commit", Name: "x"}, {Event: "pre-commit", Name: "x"}}
	})
	if got := FindClashes([]*manifest.Layer{self}); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestARoleClashIsDescribedAsARole(t *testing.T) {
	c := Clash{Kind: "capability", What: "role.agent-loop", Layers: []string{"acme/ralph-loops", "acme/goals"}}
	if got := c.String(); !strings.Contains(got, "agent-loop role") {
		t.Fatalf("%q", got)
	}
}

func TestPlanRefusesClashingLayers(t *testing.T) {
	root := fixture(t)
	mem := func(dir, id string) string {
		write(t, root, dir+"/layer.yaml", "id: "+id+"\nversion: 0.1.0\nsummary: Memory.\nprovides: [role.memory]\n")
		return root + "/" + dir
	}
	cfg := configWith(t, root, "ilk/record")
	cfg.Set(config.LayerRef{ID: "acme/claude-mem", Source: mem("l1", "acme/claude-mem")})
	cfg.Set(config.LayerRef{ID: "acme/llm-wiki", Source: mem("l2", "acme/llm-wiki")})
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Plan(PlanOptions{Prune: true})
	var ce *ClashError
	if !errors.As(err, &ce) {
		t.Fatalf("two memory layers should be refused at plan time, got %v", err)
	}
	if len(ce.Involving("acme/llm-wiki")) != 1 {
		t.Fatalf("got %+v", ce.Clashes)
	}
}
