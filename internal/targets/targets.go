// Package targets projects ilk's neutral artifacts — instructions, skills, hooks
// and commands — into the files each coding agent actually reads.
//
// Layers never emit agent-specific files. They declare what they want an agent to
// know and do; targets decide what that looks like on disk. Adding support for a
// new agent means writing an adapter here, and never touching a layer.
//
// The load-bearing rule: the CLI is the interface, and agent configuration is a
// projection of it. Every projection below ultimately tells the agent to run an
// `ilk` command, so an agent with no integration at all still gets the whole
// feature set by running the same commands a human would.
package targets

import (
	"fmt"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/manifest"
)

// Artifact is a file contribution produced by a target. It goes through the same
// plan/apply engine as a layer's own files, so targets get provenance tracking,
// conflict detection and clean removal for free.
type Artifact struct {
	Path    string
	Mode    manifest.Mode
	Region  string
	Content string
	Exec    bool
	// Owner attributes the artifact to a layer rather than to the target. It is
	// what lets several layers each own their own fenced block in one file, and
	// what makes dropping a layer remove exactly that layer's block. Empty means
	// the target owns it.
	Owner string
	// Merge co-owns a file whose format has no comment syntax to fence — JSON,
	// principally. The engine calls it with the file's current content and gets
	// back the content it should have: with ilk's entries present when adopt is
	// true, and stripped when it is false. A merged file is never deleted by
	// ilk, only vacated.
	Merge func(existing string, adopt bool) (string, error)
}

// Layer is a layer's neutral contributions, already rendered.
type Layer struct {
	ID       string
	Name     string
	Version  string
	Summary  string
	Docs     []Instruction
	Skills   []Skill
	Hooks    []manifest.Hook
	Commands []manifest.Command
	MCP      []manifest.MCPServer
}

// Instruction is rendered always-on guidance.
type Instruction struct {
	ID     string
	Body   string
	Budget int
}

// Skill is rendered on-demand guidance.
type Skill struct {
	Name        string
	Description string
	Body        string
	Layer       string
}

// Input is everything a target needs to produce its artifacts.
type Input struct {
	RepoName string
	Layers   []Layer
	// Checks lists the ids of every registered check, so a target can tell an
	// agent what it can verify.
	Checks []string
}

// AllSkills flattens skills across layers, sorted for deterministic output.
func (in Input) AllSkills() []Skill {
	var out []Skill
	for _, l := range in.Layers {
		for _, s := range l.Skills {
			s.Layer = l.ID
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// AllHooks flattens hooks for a given event across layers.
func (in Input) AllHooks(event string) []manifest.Hook {
	var out []manifest.Hook
	for _, l := range in.Layers {
		for _, h := range l.Hooks {
			if h.Event == event {
				if h.Name == "" {
					h.Name = l.Name
				}
				out = append(out, h)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// AllMCP flattens MCP servers across layers, sorted by name. Two layers
// declaring the same name is an error rather than last-write-wins, because
// every projection keys server entries on the name.
func (in Input) AllMCP() ([]manifest.MCPServer, error) {
	declaredBy := map[string]string{}
	var out []manifest.MCPServer
	for _, l := range in.Layers {
		for _, s := range l.MCP {
			if prev, ok := declaredBy[s.Name]; ok {
				return nil, fmt.Errorf("mcp server %q is declared by both %s and %s — rename one, or drop one of the layers", s.Name, prev, l.ID)
			}
			declaredBy[s.Name] = l.ID
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Target is an agent adapter.
type Target interface {
	// Name is the identifier used in .ilk/config.yaml targets.
	Name() string
	// Description is shown by `ilk doctor` and `ilk agents`.
	Description() string
	// Supports reports whether this agent can deliver a lifecycle event
	// natively. Unsupported events are not silently dropped — `ilk doctor`
	// reports them, and git hooks cover the ones that matter.
	Supports(event string) bool
	// Artifacts produces the files this target wants on disk.
	Artifacts(in Input) ([]Artifact, error)
}

// registry holds every known target, including the two that are always on.
var registry = map[string]Target{}

func register(t Target) { registry[t.Name()] = t }

func init() {
	register(agentsMD{})
	register(gitHooks{})
	register(claudeCode{})
	register(newCursor())
	register(codex{})
	register(newCopilot())
	register(newGemini())
	register(newOpencode())
}

// Always lists targets that are enabled unconditionally. AGENTS.md is the
// cross-tool standard and the fallback every other adapter points at; git hooks
// are the only enforcement that does not depend on an agent cooperating.
var Always = []string{"agents-md", "git-hooks"}

// Get returns a target by name.
func Get(name string) (Target, error) {
	t, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown agent target %q — known targets: %s", name, strings.Join(Names(), ", "))
	}
	return t, nil
}

// Names lists every known target.
func Names() []string {
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Selectable lists targets a user may enable, excluding the always-on ones.
func Selectable() []Target {
	var out []Target
	for _, name := range Names() {
		if isAlways(name) {
			continue
		}
		out = append(out, registry[name])
	}
	return out
}

func isAlways(name string) bool {
	for _, a := range Always {
		if a == name {
			return true
		}
	}
	return false
}

// Resolve returns the ordered set of targets to run: the always-on ones plus
// whatever the repository configured.
func Resolve(configured []string) ([]Target, error) {
	seen := map[string]bool{}
	var out []Target
	for _, name := range Always {
		out = append(out, registry[name])
		seen[name] = true
	}
	names := append([]string(nil), configured...)
	sort.Strings(names)
	for _, name := range names {
		if seen[name] {
			continue
		}
		t, err := Get(name)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
		seen[name] = true
	}
	return out, nil
}

// generatedNotice heads files a target owns outright.
const generatedNotice = "Generated by ilk. Do not edit — run `ilk apply` to regenerate, or edit the layer that produced it."

// pointerBody is the content of a stub that redirects an agent to AGENTS.md. The
// stub exists so that agents which look only for their own filename still find
// the instructions; it deliberately carries no content of its own, because two
// copies of the same instructions drift.
func pointerBody(agent string) string {
	return fmt.Sprintf(`This project keeps its agent instructions in **AGENTS.md** at the repository root.
Read that file — it is the source of truth for %s and every other agent working here.

The project's machine interface is the `+"`ilk`"+` command:

- `+"`ilk brief`"+` — the current state of the project, assembled from the record.
- `+"`ilk check`"+` — validate the repository; every failure prints its own fix.

Both accept `+"`--json`"+`.
`, agent)
}
