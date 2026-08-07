// Package engine computes what a repository should look like given the layers it
// has adopted, compares that against what is actually on disk, and reconciles the
// two.
//
// The shape is deliberately Terraform's: a declared desired state, a plan you can
// read before anything happens, and an apply step that is separate and explicit.
// That is what makes adopting a layer safe to try — you can always see the whole
// blast radius first, and always undo it.
package engine

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/config"
	"github.com/coflounder/ilk/internal/layer"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/coflounder/ilk/internal/render"
	"github.com/coflounder/ilk/internal/repo"
	"github.com/coflounder/ilk/internal/targets"
)

// CoreOwner labels the handful of artifacts ilk maintains for itself, regardless
// of which layers are adopted.
const CoreOwner = "ilk/core"

// TargetOwnerPrefix labels artifacts produced by an agent adapter rather than a
// layer, so `ilk agents sync` and `ilk rm` clean up only their own output.
const TargetOwnerPrefix = "target:"

// Desired is one artifact the repository should contain.
type Desired struct {
	Path    string
	Mode    manifest.Mode
	Region  string
	Content string
	Exec    bool
	Owner   string
	Merge   func(existing string, adopt bool) (string, error)
	// Dir marks a directory contract rather than a file.
	Dir bool
	// Purpose documents a directory in `ilk brief` and `ilk status`.
	Purpose string
}

// key uniquely identifies an artifact for matching against the lockfile.
func (d Desired) key() string { return d.Owner + "\x00" + d.Path + "\x00" + d.Region }

// ResolvedLayer is an adopted layer with its variables resolved and its render
// context built.
type ResolvedLayer struct {
	Ref    config.LayerRef
	Loaded *layer.Loaded
	Vars   map[string]string
	Ctx    render.Context
}

// ID is the layer's identity.
func (r *ResolvedLayer) ID() string { return r.Loaded.Manifest.ID }

// Name is the word used to dispatch `ilk <layer> <command>`.
func (r *ResolvedLayer) Name() string { return r.Loaded.Manifest.Name() }

// Project is a repository with everything ilk needs to plan against it.
type Project struct {
	Repo    *repo.Repo
	Config  *config.Config
	Lock    *lock.Lock
	Layers  []*ResolvedLayer
	Version string
}

// Load reads a repository's declared state and resolves every adopted layer.
func Load(root, version string) (*Project, error) {
	r, err := repo.Find(root)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(r.Root)
	if err != nil {
		return nil, err
	}
	lk, err := lock.Load(r.Root)
	if err != nil {
		return nil, err
	}
	p := &Project{Repo: r, Config: cfg, Lock: lk, Version: version}
	if err := p.resolveLayers(); err != nil {
		return nil, err
	}
	return p, nil
}

// NewProject builds a project from parts, for `ilk init` and for tests.
func NewProject(r *repo.Repo, cfg *config.Config, lk *lock.Lock, version string) (*Project, error) {
	p := &Project{Repo: r, Config: cfg, Lock: lk, Version: version}
	if err := p.resolveLayers(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Project) resolveLayers() error {
	cache := p.Repo.IlkPath("cache")
	p.Layers = nil
	for _, ref := range p.Config.Layers {
		src := ref.Source
		if src == "" {
			src = ref.ID
		}
		loaded, err := layer.Resolve(src, cache)
		if err != nil {
			return fmt.Errorf("layer %s: %w", ref.ID, err)
		}
		if loaded.Manifest.ID != ref.ID {
			return fmt.Errorf("layer %s resolves to %s — the id in .ilk/config.yaml and the id in the layer manifest must match", ref.ID, loaded.Manifest.ID)
		}
		vars, err := loaded.ResolveVars(ref.Vars)
		if err != nil {
			return err
		}
		p.Layers = append(p.Layers, &ResolvedLayer{Ref: ref, Loaded: loaded, Vars: vars})
	}
	// Contexts are built after every layer is known, so a layer can see its
	// neighbours without depending on them.
	ids := make([]string, 0, len(p.Layers))
	for _, l := range p.Layers {
		ids = append(ids, l.ID())
	}
	for _, l := range p.Layers {
		l.Ctx = render.Context{
			Repo:   render.RepoInfo{Name: p.Repo.Name(), Root: p.Repo.Root},
			Vars:   l.Vars,
			Caps:   p.Capabilities(),
			Layers: ids,
			Ilk:    render.IlkInfo{Version: p.Version},
		}
	}
	return nil
}

// Capabilities merges the repository's declared capabilities with those any
// layer provides. Declared values win, so a repository can always override what
// a layer assumed.
//
// A layer that supplies a value for a capability has it rendered here, against a
// context holding only its own variables. That restriction is what keeps this
// function from needing the answer it is computing: a capability value may not
// read another capability.
func (p *Project) Capabilities() map[string]string {
	caps := map[string]string{}
	for _, l := range p.Layers {
		for _, c := range l.Loaded.Manifest.Provides.Names() {
			if _, ok := caps[c]; ok {
				continue
			}
			tmpl := l.Loaded.Manifest.Provides[c]
			if tmpl == "" {
				// No value: the capability is a bare declaration, and naming the
				// provider is more useful to a reader than an empty string.
				caps[c] = l.ID()
				continue
			}
			value, err := render.String(l.ID()+":provides:"+c, tmpl, p.provideCtx(l))
			if err != nil {
				// A malformed value must not take down every command that reads
				// capabilities. `ilk check` reports it through the manifest.
				caps[c] = l.ID()
				continue
			}
			caps[c] = strings.TrimSpace(value)
		}
	}
	for k, v := range p.Config.Capabilities {
		caps[k] = v
	}
	return caps
}

// provideCtx is the reduced context a `provides:` value is rendered against.
func (p *Project) provideCtx(l *ResolvedLayer) render.Context {
	return render.Context{
		Repo: render.RepoInfo{Name: p.Repo.Name(), Root: p.Repo.Root},
		Vars: l.Vars,
		Caps: map[string]string{},
		Ilk:  render.IlkInfo{Version: p.Version},
	}
}

// MissingRequirements lists capabilities an adopted layer needs that nothing
// supplies.
func (p *Project) MissingRequirements() map[string][]string {
	caps := p.Capabilities()
	missing := map[string][]string{}
	for _, l := range p.Layers {
		for _, req := range l.Loaded.Manifest.Requires {
			if _, ok := caps[req]; !ok {
				missing[l.ID()] = append(missing[l.ID()], req)
			}
		}
	}
	return missing
}

// Layer finds an adopted layer by id or short name.
func (p *Project) Layer(nameOrID string) (*ResolvedLayer, bool) {
	for _, l := range p.Layers {
		if l.ID() == nameOrID || l.Name() == nameOrID {
			return l, true
		}
	}
	return nil, false
}

// Desired computes every artifact the repository should contain.
func (p *Project) Desired() ([]Desired, error) {
	var out []Desired

	out = append(out, p.coreDesired()...)

	for _, l := range p.Layers {
		items, err := p.layerDesired(l)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}

	groupItems, err := p.groupDesired()
	if err != nil {
		return nil, err
	}
	out = append(out, groupItems...)

	targetItems, err := p.targetDesired()
	if err != nil {
		return nil, err
	}
	out = append(out, targetItems...)

	if err := checkCollisions(out); err != nil {
		return nil, err
	}
	sortDesired(out)
	return out, nil
}

// coreDesired is what ilk maintains for itself in every repository.
func (p *Project) coreDesired() []Desired {
	return []Desired{{
		Path:   ".gitignore",
		Mode:   manifest.ModeAppendOnce,
		Region: "core",
		Owner:  CoreOwner,
		Content: "# ilk keeps fetched layer sources here; they are rebuildable.\n" +
			".ilk/cache/\n",
	}}
}

// dirPath resolves a directory contract to a repository-relative path. Both the
// group and the name render, so a repository can still rename either.
func (p *Project) dirPath(l *ResolvedLayer, d manifest.Dir) (string, error) {
	if !d.Grouped() {
		return render.Path(d.Path, l.Ctx)
	}
	group, err := render.Path(d.Group, l.Ctx)
	if err != nil {
		return "", err
	}
	name, err := render.Path(d.Name, l.Ctx)
	if err != nil {
		return "", err
	}
	if group == "" {
		return name, nil
	}
	return path.Join(group, name), nil
}

// Group is a resolved grouping with the directories that landed in it.
type Group struct {
	Name    string
	Purpose string
	Order   int
	Members []GroupMember
}

// GroupMember is one directory inside a group.
type GroupMember struct {
	Name    string
	Path    string
	Purpose string
	Order   int
	Owner   string
}

// Groups resolves every grouping the repository's layers declare, together with
// the directories that belong to each.
//
// Where two layers declare the same group, the first by layer id wins and a
// canonical declaration always wins — deliberately forgiving, because two layers
// describing `infra` in slightly different words is not a problem worth failing
// a plan over.
func (p *Project) Groups() ([]Group, error) {
	byName := map[string]*Group{}
	for _, g := range manifest.CanonicalGroups {
		byName[g.Name] = &Group{Name: g.Name, Purpose: g.Purpose, Order: g.Order}
	}

	layers := append([]*ResolvedLayer(nil), p.Layers...)
	sort.Slice(layers, func(i, j int) bool { return layers[i].ID() < layers[j].ID() })

	for _, l := range layers {
		for _, g := range l.Loaded.Manifest.Groups {
			if manifest.IsCanonicalGroup(g.Name) {
				continue
			}
			if _, taken := byName[g.Name]; taken {
				continue
			}
			purpose, err := render.String(l.ID()+":group:"+g.Name, g.Purpose, l.Ctx)
			if err != nil {
				return nil, err
			}
			byName[g.Name] = &Group{Name: g.Name, Purpose: purpose, Order: g.Order}
		}
	}

	for _, l := range layers {
		for _, d := range l.Loaded.Manifest.Dirs {
			if !d.Grouped() {
				continue
			}
			full, err := p.dirPath(l, d)
			if err != nil {
				return nil, err
			}
			group, err := render.Path(d.Group, l.Ctx)
			if err != nil {
				return nil, err
			}
			g, ok := byName[group]
			if !ok {
				return nil, fmt.Errorf("layer %s puts %s in group %q, which nothing declares — add a `groups:` entry for it in that layer", l.ID(), full, group)
			}
			purpose, err := render.String(l.ID()+":purpose:"+full, d.Purpose, l.Ctx)
			if err != nil {
				return nil, err
			}
			g.Members = append(g.Members, GroupMember{
				Name: path.Base(full), Path: full, Purpose: purpose, Order: d.Order, Owner: l.ID(),
			})
		}
	}

	var out []Group
	for _, g := range byName {
		if len(g.Members) == 0 {
			// A group nothing uses is not a directory; it is just a word ilk knows.
			continue
		}
		sort.SliceStable(g.Members, func(i, j int) bool {
			if g.Members[i].Order != g.Members[j].Order {
				return g.Members[i].Order < g.Members[j].Order
			}
			return g.Members[i].Name < g.Members[j].Name
		})
		out = append(out, *g)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// groupDesired writes each group's index: the ordered list of what is inside it
// and what each part holds.
//
// This is where directory ordering lives. An index can be reordered for free, it
// carries each directory's stated purpose, and it does not put a number in a path
// that links and `covers:` globs would then have to track.
func (p *Project) groupDesired() ([]Desired, error) {
	groups, err := p.Groups()
	if err != nil {
		return nil, err
	}
	var out []Desired
	for _, g := range groups {
		var b strings.Builder
		if g.Purpose != "" {
			b.WriteString(g.Purpose)
			b.WriteString("\n\n")
		}
		b.WriteString("| Directory | Holds |\n|---|---|\n")
		for _, m := range g.Members {
			purpose := m.Purpose
			if strings.TrimSpace(purpose) == "" {
				purpose = "—"
			}
			fmt.Fprintf(&b, "| [`%s/`](%s/) | %s |\n", m.Name, m.Name, purpose)
		}
		out = append(out, Desired{
			Path:    path.Join(g.Name, "README.md"),
			Mode:    manifest.ModeRegion,
			Region:  "index",
			Owner:   CoreOwner,
			Content: b.String(),
		})
	}
	return out, nil
}

func (p *Project) layerDesired(l *ResolvedLayer) ([]Desired, error) {
	m := l.Loaded.Manifest
	var out []Desired
	var ignores []string

	for _, d := range m.Dirs {
		path, err := p.dirPath(l, d)
		if err != nil {
			return nil, err
		}
		purpose, err := render.String("purpose", d.Purpose, l.Ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, Desired{Path: path, Dir: true, Owner: l.ID(), Purpose: purpose})
		if d.Keep {
			out = append(out, Desired{
				Path:    strings.TrimSuffix(path, "/") + "/.gitkeep",
				Mode:    manifest.ModeManaged,
				Owner:   l.ID(),
				Content: "",
			})
		}
		if d.Ignore {
			ignores = append(ignores, strings.TrimSuffix(path, "/")+"/")
		}
	}

	if len(ignores) > 0 {
		sort.Strings(ignores)
		out = append(out, Desired{
			Path:    ".gitignore",
			Mode:    manifest.ModeAppendOnce,
			Region:  "ignore",
			Owner:   l.ID(),
			Content: strings.Join(ignores, "\n") + "\n",
		})
	}

	for i, f := range m.Files {
		include, err := render.Truthy(f.When, l.Ctx)
		if err != nil {
			return nil, fmt.Errorf("layer %s files[%d] when: %w", l.ID(), i, err)
		}
		if !include {
			continue
		}
		dest, err := render.Path(f.Dest, l.Ctx)
		if err != nil {
			return nil, err
		}
		body := f.Inline
		if f.Src != "" {
			body, err = l.Loaded.ReadSource(f.Src)
			if err != nil {
				return nil, err
			}
		}
		content, err := render.String(l.ID()+":"+f.Dest, body, l.Ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, Desired{
			Path:    dest,
			Mode:    f.Mode,
			Region:  f.Region,
			Owner:   l.ID(),
			Content: content,
			Exec:    f.Exec,
		})
	}

	return out, nil
}

// targetInput builds the neutral artifact set every agent adapter projects from.
func (p *Project) targetInput() (targets.Input, error) {
	in := targets.Input{RepoName: p.Repo.Name()}
	for _, l := range p.Layers {
		m := l.Loaded.Manifest
		tl := targets.Layer{
			ID:       m.ID,
			Name:     m.Name(),
			Version:  m.Version,
			Summary:  m.Summary,
			Hooks:    m.Hooks,
			Commands: m.Commands,
		}
		for _, ins := range m.Instructions {
			body := ins.Inline
			if ins.Src != "" {
				var err error
				body, err = l.Loaded.ReadSource(ins.Src)
				if err != nil {
					return in, err
				}
			}
			rendered, err := render.String(m.ID+":instructions:"+ins.ID, body, l.Ctx)
			if err != nil {
				return in, err
			}
			tl.Docs = append(tl.Docs, targets.Instruction{ID: ins.ID, Body: rendered, Budget: ins.Budget})
		}
		for _, s := range m.Skills {
			body := s.Inline
			if s.Src != "" {
				var err error
				body, err = l.Loaded.ReadSource(s.Src)
				if err != nil {
					return in, err
				}
			}
			rendered, err := render.String(m.ID+":skill:"+s.Name, body, l.Ctx)
			if err != nil {
				return in, err
			}
			desc, err := render.String(m.ID+":skilldesc:"+s.Name, s.Description, l.Ctx)
			if err != nil {
				return in, err
			}
			tl.Skills = append(tl.Skills, targets.Skill{Name: s.Name, Description: desc, Body: rendered, Layer: m.ID})
		}
		in.Layers = append(in.Layers, tl)
	}
	for _, c := range p.CheckIDs() {
		in.Checks = append(in.Checks, c)
	}
	return in, nil
}

func (p *Project) targetDesired() ([]Desired, error) {
	in, err := p.targetInput()
	if err != nil {
		return nil, err
	}
	ts, err := targets.Resolve(p.Config.Targets)
	if err != nil {
		return nil, err
	}
	var out []Desired
	for _, t := range ts {
		artifacts, err := t.Artifacts(in)
		if err != nil {
			return nil, fmt.Errorf("target %s: %w", t.Name(), err)
		}
		for _, a := range artifacts {
			owner := a.Owner
			if owner == "" {
				owner = TargetOwnerPrefix + t.Name()
			}
			out = append(out, Desired{
				Path:    a.Path,
				Mode:    a.Mode,
				Region:  a.Region,
				Owner:   owner,
				Content: a.Content,
				Exec:    a.Exec,
				Merge:   a.Merge,
			})
		}
	}
	return out, nil
}

// CheckIDs lists every check registered by an adopted layer.
func (p *Project) CheckIDs() []string {
	var ids []string
	for _, l := range p.Layers {
		for _, c := range l.Loaded.Manifest.Checks {
			ids = append(ids, c.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// checkCollisions rejects a configuration where two owners claim the same file
// outright. Two layers each owning a region in one file is fine and expected;
// two layers both claiming to own the whole file is not, and silently letting the
// last one win would be worse than refusing.
func checkCollisions(items []Desired) error {
	type claim struct{ owner, mode string }
	seen := map[string]claim{}
	for _, d := range items {
		if d.Dir || d.Mode == manifest.ModeRegion || d.Mode == manifest.ModeAppendOnce || d.Mode == manifest.ModeMerge {
			continue
		}
		if prev, ok := seen[d.Path]; ok && prev.owner != d.Owner {
			return fmt.Errorf("%s is claimed by both %s and %s — one of them must use a fenced region instead of owning the whole file", d.Path, prev.owner, d.Owner)
		}
		seen[d.Path] = claim{d.Owner, string(d.Mode)}
	}
	return nil
}

// modeRank orders operations on the same path so a file is seeded before regions
// are inserted into it.
func modeRank(d Desired) int {
	switch {
	case d.Dir:
		return 0
	case d.Mode == manifest.ModeCreateOnly:
		return 1
	case d.Mode == manifest.ModeManaged:
		return 2
	case d.Mode == manifest.ModeMerge:
		return 3
	case d.Mode == manifest.ModeRegion, d.Mode == manifest.ModeAppendOnce:
		return 4
	}
	return 5
}

func sortDesired(items []Desired) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		// Directories first overall, so parents exist before children are written.
		if (a.Dir) != (b.Dir) {
			return a.Dir
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if ra, rb := modeRank(a), modeRank(b); ra != rb {
			return ra < rb
		}
		if a.Owner != b.Owner {
			return a.Owner < b.Owner
		}
		return a.Region < b.Region
	})
}

// TargetInput exposes the neutral artifact set for callers outside the engine —
// the budget check, and `ilk brief`.
func (p *Project) TargetInput() (targets.Input, error) { return p.targetInput() }
