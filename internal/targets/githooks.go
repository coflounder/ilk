package targets

import (
	"fmt"

	"github.com/coflounder/ilk/internal/manifest"
)

// gitHooks is the universal substrate, and the reason ilk's gates do not depend
// on an agent choosing to cooperate.
//
// Agent-native hooks make feedback faster; these make it enforceable. An agent
// that ignores every instruction in AGENTS.md still cannot get a commit past a
// failing pre-commit hook.
type gitHooks struct{}

func (gitHooks) Name() string { return "git-hooks" }
func (gitHooks) Description() string {
	return "git hooks — enforcement that does not rely on the agent"
}

func (gitHooks) Supports(event string) bool {
	return event == "pre-commit" || event == "pre-push"
}

func (t gitHooks) Artifacts(in Input) ([]Artifact, error) {
	var out []Artifact
	for _, ev := range []string{"pre-commit", "pre-push"} {
		if len(in.AllHooks(ev)) == 0 {
			continue
		}
		out = append(out, Artifact{
			Path:    ".git/hooks/" + ev,
			Mode:    manifest.ModeManaged,
			Exec:    true,
			Content: gitHookScript(ev),
		})
	}
	return out, nil
}

// gitHookScript delegates to `ilk hook run`, which is the only entrypoint any
// adapter writes. Layers can add and remove hooks without this file changing.
//
// The script exits 0 when ilk is not installed. A missing tool should not lock a
// contributor out of their own repository; `ilk doctor` is where that gets
// reported.
func gitHookScript(event string) string {
	return fmt.Sprintf(`#!/bin/sh
# %s
#
# Runs the %s hooks declared by the layers in .ilk/config.yaml.
# Bypass in an emergency with `+"`git commit --no-verify`"+`.
set -eu

if ! command -v ilk >/dev/null 2>&1; then
	echo "ilk: not installed — skipping %s hooks (see https://github.com/coflounder/ilk)" >&2
	exit 0
fi

exec ilk hook run %s
`, generatedNotice, event, event, event)
}

// pointer targets emit only a redirect to AGENTS.md. Duplicating instructions
// into every agent's preferred filename guarantees they drift apart; a pointer
// cannot.
type pointerTarget struct {
	name  string
	desc  string
	path  string
	agent string
}

func (p pointerTarget) Name() string         { return p.name }
func (p pointerTarget) Description() string  { return p.desc }
func (p pointerTarget) Supports(string) bool { return false }

func (p pointerTarget) Artifacts(Input) ([]Artifact, error) {
	return []Artifact{{
		Path:    p.path,
		Mode:    manifest.ModeRegion,
		Region:  "pointer",
		Content: pointerBody(p.agent),
	}}, nil
}

type cursor struct{ pointerTarget }
type copilot struct{ pointerTarget }
type gemini struct{ pointerTarget }
type opencode struct{ pointerTarget }

func newCursor() cursor {
	return cursor{pointerTarget{
		name:  "cursor",
		desc:  "Cursor — a rule file pointing at AGENTS.md",
		path:  ".cursor/rules/ilk.mdc",
		agent: "Cursor",
	}}
}

func newCopilot() copilot {
	return copilot{pointerTarget{
		name:  "copilot",
		desc:  "GitHub Copilot — repository instructions pointing at AGENTS.md",
		path:  ".github/copilot-instructions.md",
		agent: "GitHub Copilot",
	}}
}

func newGemini() gemini {
	return gemini{pointerTarget{
		name:  "gemini",
		desc:  "Gemini CLI — GEMINI.md pointing at AGENTS.md",
		path:  "GEMINI.md",
		agent: "the Gemini CLI",
	}}
}

func newOpencode() opencode {
	return opencode{pointerTarget{
		name:  "opencode",
		desc:  "OpenCode — skills mirrored from .agent/skills",
		path:  ".opencode/AGENTS.md",
		agent: "OpenCode",
	}}
}

// Cursor rule files need frontmatter to be applied automatically.
func (c cursor) Artifacts(in Input) ([]Artifact, error) {
	body := "---\nalwaysApply: true\ndescription: Project instructions\n---\n\n" + pointerBody("Cursor")
	return []Artifact{{
		Path:    c.path,
		Mode:    manifest.ModeManaged,
		Content: body,
	}}, nil
}

// codex reads AGENTS.md natively, so there is nothing to project. It is still a
// selectable target so that `ilk doctor` can say so out loud rather than leaving
// a user wondering whether they forgot a step.
type codex struct{}

func (codex) Name() string         { return "codex" }
func (codex) Description() string  { return "Codex — reads AGENTS.md natively; nothing to generate" }
func (codex) Supports(string) bool { return false }

func (codex) Artifacts(Input) ([]Artifact, error) { return nil, nil }

// Skills for agents that read a directory of markdown but have no frontmatter
// convention: mirror the canonical files.
func (o opencode) Artifacts(in Input) ([]Artifact, error) {
	var out []Artifact
	out = append(out, Artifact{
		Path:    o.path,
		Mode:    manifest.ModeManaged,
		Content: pointerBody("OpenCode"),
	})
	for _, s := range in.AllSkills() {
		out = append(out, Artifact{
			Path:    fmt.Sprintf(".opencode/skills/%s/SKILL.md", s.Name),
			Mode:    manifest.ModeManaged,
			Content: skillDocument(s),
		})
	}
	return out, nil
}
