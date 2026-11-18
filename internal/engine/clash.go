package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/manifest"
)

// Clash is two or more layers trying to do the same job in one repository.
//
// Most of what layers contribute stacks: checks add up, git hooks run in turn,
// and every layer writes its own fenced block in AGENTS.md. What does not stack
// is anything a repository can have only one of — the provider of a capability,
// a skill's name, a check id, an MCP server, a hook's name, or the word that
// dispatches `ilk <name>`. Jobs only one layer may hold, like an agent's loop or
// its memory, are roles (see manifest.Roles), and a role is a capability.
//
// Letting the first layer win there, as ilk used to for capabilities, means one
// layer silently does nothing while its author believes it is working.
type Clash struct {
	// Kind is what is claimed twice: capability, skill, check, mcp, name or
	// hook.
	Kind string `json:"kind"`
	// What is the contested name: the capability, the skill, the check id.
	What string `json:"what"`
	// Layers are the claimants, in the order the configuration lists them.
	Layers []string `json:"layers"`
}

func (c Clash) String() string {
	both := strings.Join(c.Layers, " and ")
	switch c.Kind {
	case "capability":
		if manifest.IsRole(c.What) {
			return fmt.Sprintf("%s both fill the %s role — a repository has one", both, strings.TrimPrefix(c.What, manifest.RolePrefix))
		}
		return fmt.Sprintf("%s both provide %s — a repository has one provider of each capability", both, c.What)
	case "skill":
		return fmt.Sprintf("%s both ship a skill called %q", both, c.What)
	case "check":
		return fmt.Sprintf("%s both register the check %q", both, c.What)
	case "mcp":
		return fmt.Sprintf("%s both declare the MCP server %q", both, c.What)
	case "name":
		return fmt.Sprintf("%s are both called %q, so `ilk %s` could mean either", both, c.What, c.What)
	case "hook":
		event, name, _ := strings.Cut(c.What, " ")
		return fmt.Sprintf("%s both register a %s hook called %q", both, event, name)
	}
	return fmt.Sprintf("%s clash over %s %s", both, c.Kind, c.What)
}

// ClashError is a configuration ilk refuses to plan.
type ClashError struct {
	Clashes []Clash
}

func (e *ClashError) Error() string {
	lines := make([]string, len(e.Clashes))
	for i, c := range e.Clashes {
		lines[i] = c.String()
	}
	if len(lines) == 1 {
		return lines[0]
	}
	return "layers clash:\n  " + strings.Join(lines, "\n  ")
}

// Fix says how to get out of the clash.
func (e *ClashError) Fix() string {
	return "keep one of each: `ilk rm <layer>`, or `ilk add <layer> --replace` to swap the one that is here for the one you want"
}

// Involving reports the clashes that name the layer.
func (e *ClashError) Involving(id string) []Clash {
	var out []Clash
	for _, c := range e.Clashes {
		for _, l := range c.Layers {
			if l == id {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// FindClashes lists every clash among the given layers. A layer never clashes
// with itself.
func FindClashes(layers []*manifest.Layer) []Clash {
	type key struct{ kind, what string }
	claims := map[key][]string{}
	var order []key
	claim := func(kind, what, id string) {
		k := key{kind, what}
		for _, prev := range claims[k] {
			if prev == id {
				return
			}
		}
		if _, ok := claims[k]; !ok {
			order = append(order, k)
		}
		claims[k] = append(claims[k], id)
	}

	for _, m := range layers {
		for _, c := range m.Provides.Names() {
			claim("capability", c, m.ID)
		}
		for _, s := range m.Skills {
			claim("skill", s.Name, m.ID)
		}
		for _, c := range m.Checks {
			claim("check", c.ID, m.ID)
		}
		for _, s := range m.MCP {
			claim("mcp", s.Name, m.ID)
		}
		claim("name", m.Name(), m.ID)
		for _, h := range m.Hooks {
			// Hooks on one event are meant to stack — several layers check a
			// commit, and blocking turn-end hooks compose into "not done until
			// all of these pass". Only a hook's name must be unique, because it
			// is how a failure is reported and how a hook is listed.
			if h.Name != "" {
				claim("hook", h.Event+" "+h.Name, m.ID)
			}
		}
	}

	var out []Clash
	for _, k := range order {
		if ids := claims[k]; len(ids) > 1 {
			out = append(out, Clash{Kind: k.kind, What: k.what, Layers: ids})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].What < out[j].What
	})
	return out
}

// Clashes lists the clashes among this project's layers.
func (p *Project) Clashes() []Clash {
	ms := make([]*manifest.Layer, 0, len(p.Layers))
	for _, l := range p.Layers {
		ms = append(ms, l.Loaded.Manifest)
	}
	return FindClashes(ms)
}
