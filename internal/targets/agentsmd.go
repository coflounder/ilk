package targets

import (
	"fmt"
	"strings"

	"github.com/coflounder/ilk/internal/manifest"
)

// agentsMD writes AGENTS.md, the cross-tool standard read natively by most
// coding agents and stewarded by the Linux Foundation's Agentic AI Foundation.
// It is always on: it is both the canonical instruction surface and the fallback
// every other adapter points back at.
type agentsMD struct{}

func (agentsMD) Name() string        { return "agents-md" }
func (agentsMD) Description() string { return "AGENTS.md — the cross-tool instruction standard" }

// AGENTS.md is a file an agent reads, not an event delivery mechanism.
func (agentsMD) Supports(string) bool { return false }

func (t agentsMD) Artifacts(in Input) ([]Artifact, error) {
	var out []Artifact

	// The header is seeded once and then belongs to whoever maintains the repo.
	// ilk only ever owns the fenced regions below it.
	out = append(out, Artifact{
		Path: "AGENTS.md",
		Mode: manifest.ModeCreateOnly,
		Content: fmt.Sprintf(`# %s

<!--
  Instructions for coding agents working in this repository.

  Prose outside the ilk:begin / ilk:end blocks is yours: describe what this project
  is, how to build and test it, and the non-obvious things a newcomer gets wrong.
  Keep it short and specific — restating what the repository already shows measurably
  makes agents worse, not better.

  The fenced blocks below are generated from the layers in .ilk/config.yaml.
-->

`, in.RepoName),
	})

	// One region per layer, so adopting and dropping a layer adds and removes
	// exactly its own guidance.
	for _, l := range in.Layers {
		if len(l.Docs) == 0 {
			continue
		}
		var b strings.Builder
		for i, d := range l.Docs {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(strings.TrimRight(d.Body, "\n"))
			b.WriteString("\n")
		}
		out = append(out, Artifact{
			Path:    "AGENTS.md",
			Mode:    manifest.ModeRegion,
			Region:  "instructions",
			Owner:   l.ID,
			Content: b.String(),
		})
	}

	// A single index of on-demand skills. Agents without native skill support
	// still discover them here, which is what makes skills portable rather than
	// a Claude Code feature.
	if skills := in.AllSkills(); len(skills) > 0 {
		var b strings.Builder
		b.WriteString("## Skills\n\n")
		b.WriteString("Detailed procedures live in files, not in this document. Read the matching\n")
		b.WriteString("file when its situation applies — do not read them all up front.\n\n")
		for _, s := range skills {
			fmt.Fprintf(&b, "- **%s** — %s\n  `.agents/skills/%s/SKILL.md`\n", s.Name, s.Description, s.Name)
		}
		out = append(out, Artifact{
			Path:    "AGENTS.md",
			Mode:    manifest.ModeRegion,
			Region:  "skills",
			Content: b.String(),
		})
	}

	// Canonical skill files. Every other target either symlinks, copies or points
	// at these.
	for _, s := range in.AllSkills() {
		out = append(out, Artifact{
			Path:    fmt.Sprintf(".agents/skills/%s/SKILL.md", s.Name),
			Mode:    manifest.ModeManaged,
			Content: skillDocument(s),
		})
	}

	return out, nil
}

// skillDocument renders a skill with the YAML frontmatter that skill-aware agents
// expect, and that skill-unaware agents harmlessly ignore.
func skillDocument(s Skill) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", s.Name)
	fmt.Fprintf(&b, "description: %s\n", yamlScalar(s.Description))
	fmt.Fprintf(&b, "# %s\n", generatedNotice)
	fmt.Fprintf(&b, "ilk_layer: %s\n", s.Layer)
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimRight(s.Body, "\n"))
	b.WriteString("\n")
	return b.String()
}

// yamlScalar quotes a value when plain style would misparse it.
func yamlScalar(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if strings.ContainsAny(s, `:#{}[]&*!|>'"%@`+"`") {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
	}
	return s
}
