package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/config"
)

// The brief leaves out the skills list when AGENTS.md already carries it, and
// keeps it whenever AGENTS.md is behind — the session would otherwise start
// with no list at all.

const skilledLayer = `id: test/skilled
version: 0.1.0
summary: Skill fixture.
skills:
  - name: fold-the-towels
    description: Use when towels need folding.
    inline: Fold them in thirds.
`

func skilledRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	layerDir := filepath.Join(root, "layer")
	mustMkdir(t, layerDir)
	mustWrite(t, filepath.Join(layerDir, "layer.yaml"), skilledLayer)
	mustMkdir(t, filepath.Join(root, ".ilk"))
	cfg := config.Default()
	cfg.Agents = nil
	cfg.Set(config.LayerRef{ID: "test/skilled", Source: layerDir})
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	return root
}

type briefJSON struct {
	Skills      []struct{ Name string } `json:"skills"`
	SkillsIndex string                  `json:"skills_index"`
}

func hasSkill(b briefJSON, name string) bool {
	for _, s := range b.Skills {
		if s.Name == name {
			return true
		}
	}
	return false
}

func readBrief(t *testing.T, root string) (string, briefJSON) {
	t.Helper()
	code, text, stderr := invoke(t, "-C", root, "brief")
	if code != exitOK {
		t.Fatalf("brief: exit %d\n%s", code, stderr)
	}
	code, raw, stderr := invoke(t, "-C", root, "brief", "--json")
	if code != exitOK {
		t.Fatalf("brief --json: exit %d\n%s", code, stderr)
	}
	var b briefJSON
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		t.Fatal(err)
	}
	return text, b
}

func TestBriefDropsTheSkillsListOnceAgentsMDHasIt(t *testing.T) {
	root := skilledRepo(t)
	if code, _, stderr := invoke(t, "-C", root, "apply", "--yes"); code != exitOK {
		t.Fatalf("apply: exit %d\n%s", code, stderr)
	}
	if agents := readFile(t, filepath.Join(root, "AGENTS.md")); !strings.Contains(agents, "fold-the-towels") {
		t.Fatalf("AGENTS.md should list the skill:\n%s", agents)
	}

	text, b := readBrief(t, root)
	if strings.Contains(text, "fold-the-towels") {
		t.Errorf("the brief repeats a skills list AGENTS.md already carries:\n%s", text)
	}
	if !strings.Contains(text, "Listed under Skills in AGENTS.md") {
		t.Errorf("the brief should say where the skills are listed:\n%s", text)
	}
	if b.SkillsIndex != "AGENTS.md" || len(b.Skills) != 0 {
		t.Errorf("JSON: skills_index %q, %d skills; want AGENTS.md and none", b.SkillsIndex, len(b.Skills))
	}
}

func TestBriefKeepsTheSkillsListWhenAgentsMDIsBehind(t *testing.T) {
	root := skilledRepo(t)

	// Never applied: there is no AGENTS.md to read the list from.
	text, b := readBrief(t, root)
	if !strings.Contains(text, "fold-the-towels") || b.SkillsIndex != "" || !hasSkill(b, "fold-the-towels") {
		t.Errorf("before apply the brief must list the skills itself: index %q, skills %v\n%s", b.SkillsIndex, b.Skills, text)
	}

	// Applied, then the block was edited: AGENTS.md no longer says what ilk would.
	if code, _, stderr := invoke(t, "-C", root, "apply", "--yes"); code != exitOK {
		t.Fatalf("apply: exit %d\n%s", code, stderr)
	}
	path := filepath.Join(root, "AGENTS.md")
	edited := strings.Replace(readFile(t, path), "fold-the-towels", "fold-the-napkins", 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	text, b = readBrief(t, root)
	if !strings.Contains(text, "fold-the-towels") || b.SkillsIndex != "" {
		t.Errorf("with a stale AGENTS.md the brief must list the skills itself: index %q\n%s", b.SkillsIndex, text)
	}
}
