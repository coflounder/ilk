package targets

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coflounder/ilk/internal/manifest"
)

// claudeCode projects into Claude Code's native surfaces: skills it can load on
// demand, slash commands that shell out to ilk, and hooks in settings.json.
//
// Nothing here carries content of its own. The skills are symlinks to the
// canonical bodies in .agents/skills, and every command and hook invokes the CLI,
// so a repository configured for Claude Code behaves identically for an agent
// that has never heard of it — and a skill is only ever edited in one place.
type claudeCode struct{}

func (claudeCode) Name() string        { return "claude-code" }
func (claudeCode) Description() string { return "Claude Code — skills, slash commands, hooks" }

func (claudeCode) Supports(event string) bool {
	switch event {
	case "session-start", "post-edit", "pre-tool-use":
		return true
	}
	return false
}

func (t claudeCode) Artifacts(in Input) ([]Artifact, error) {
	var out []Artifact

	out = append(out, Artifact{
		Path:    "CLAUDE.md",
		Mode:    manifest.ModeRegion,
		Region:  "pointer",
		Content: pointerBody("Claude Code"),
	})

	// One link per skill, not one link for the directory. `.claude/skills/` is a
	// shared namespace — a person may keep hand-written skills there — so ilk
	// takes exactly the entries it owns and leaves the rest of the directory alone.
	for _, s := range in.AllSkills() {
		out = append(out, Artifact{
			Path:    fmt.Sprintf(".claude/skills/%s", s.Name),
			Mode:    manifest.ModeSymlink,
			Content: fmt.Sprintf("../../.agents/skills/%s", s.Name),
		})
	}

	// Slash commands are thin wrappers. `ilk <layer> <command>` remains the real
	// interface; this only saves typing.
	for _, l := range in.Layers {
		for _, c := range l.Commands {
			out = append(out, Artifact{
				Path:    fmt.Sprintf(".claude/commands/%s-%s.md", l.Name, c.Name),
				Mode:    manifest.ModeManaged,
				Content: slashCommand(l.Name, c),
			})
		}
	}

	// settings.json belongs to the user. ilk co-owns it: it inserts one entry per
	// event it needs and strips exactly those entries again on removal, leaving
	// every other setting untouched.
	events := t.activeEvents(in)
	out = append(out, Artifact{
		Path: ".claude/settings.json",
		Mode: manifest.ModeMerge,
		Merge: func(existing string, adopt bool) (string, error) {
			return mergeClaudeSettings(existing, events, adopt)
		},
	})

	// .mcp.json likewise belongs to the user; ilk contributes one entry per
	// declared server, each an `ilk mcp run` indirection. See mcp.go.
	mcp, err := mcpArtifact(".mcp.json", in)
	if err != nil {
		return nil, err
	}
	out = append(out, mcp)

	return out, nil
}

// activeEvents lists the events Claude Code can deliver and some adopted layer
// actually uses.
func (t claudeCode) activeEvents(in Input) []string {
	var events []string
	for _, ev := range manifest.Events {
		if t.Supports(ev) && len(in.AllHooks(ev)) > 0 {
			events = append(events, ev)
		}
	}
	return events
}

func slashCommand(layer string, c manifest.Command) string {
	return fmt.Sprintf(`---
description: %s
# %s
---

Run this and report what it printed:

`+"```"+`
ilk %s %s $ARGUMENTS
`+"```"+`
`, yamlScalar(c.Summary), generatedNotice, layer, c.Name)
}

// hookCommand is the single entrypoint every adapter writes. Layers add and
// remove hooks freely; the generated agent configuration never changes, because
// it always says the same thing: ask ilk what to run.
func hookCommand(event string) string {
	return "ilk hook run " + event
}

// isIlkHookCommand identifies an entry ilk wrote. Matching on the command string
// means ilk needs no marker keys in a file whose schema it does not own.
func isIlkHookCommand(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "ilk hook run ")
}

func claudeEventName(event string) (name, matcher string) {
	switch event {
	case "session-start":
		return "SessionStart", ""
	case "post-edit":
		return "PostToolUse", "Edit|Write|MultiEdit"
	case "pre-tool-use":
		return "PreToolUse", "*"
	}
	return "", ""
}

// mergeClaudeSettings inserts or removes ilk's hook entries in a Claude Code
// settings document, preserving everything else byte-for-byte where it can and
// structurally where it cannot.
func mergeClaudeSettings(existing string, events []string, adopt bool) (string, error) {
	doc := map[string]any{}
	if strings.TrimSpace(existing) != "" {
		if err := json.Unmarshal([]byte(existing), &doc); err != nil {
			return "", fmt.Errorf(".claude/settings.json is not valid JSON, so ilk will not touch it: %w", err)
		}
	}

	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	// Always strip ilk's own entries first, so this function is idempotent and
	// removal is just "strip, then do not re-add".
	for name, raw := range hooks {
		list, ok := raw.([]any)
		if !ok {
			continue
		}
		kept := make([]any, 0, len(list))
		for _, item := range list {
			if !claudeGroupIsIlk(item) {
				kept = append(kept, item)
			}
		}
		if len(kept) == 0 {
			delete(hooks, name)
		} else {
			hooks[name] = kept
		}
	}

	if adopt {
		for _, ev := range events {
			name, matcher := claudeEventName(ev)
			if name == "" {
				continue
			}
			group := map[string]any{
				"hooks": []any{map[string]any{
					"type":    "command",
					"command": hookCommand(ev),
				}},
			}
			if matcher != "" {
				group["matcher"] = matcher
			}
			list, _ := hooks[name].([]any)
			hooks[name] = append(list, group)
		}
	}

	if len(hooks) == 0 {
		delete(doc, "hooks")
	} else {
		doc["hooks"] = hooks
	}

	if len(doc) == 0 {
		return "", nil
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out) + "\n", nil
}

// claudeGroupIsIlk reports whether a hook group was written by ilk.
func claudeGroupIsIlk(item any) bool {
	group, ok := item.(map[string]any)
	if !ok {
		return false
	}
	inner, ok := group["hooks"].([]any)
	if !ok {
		return false
	}
	for _, h := range inner {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		if cmd, ok := hm["command"].(string); ok && isIlkHookCommand(cmd) {
			return true
		}
	}
	return false
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
