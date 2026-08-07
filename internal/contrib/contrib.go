// Package contrib turns what a repository learned about a layer into a proposal
// its maintainer can judge.
//
// The problem it solves is decay. A layer arrives with opinions, a repository
// finds one of them wrong, somebody edits the managed file, and the edit works —
// so nobody ever tells upstream. The layer keeps shipping the wrong opinion and
// the next hundred adopters make the same edit. Contribution is not politeness
// here; it is the only mechanism by which a published practice gets better.
//
// ilk is unusually well placed to do this, because it already knows exactly what
// it delivered. The lockfile holds what the layer produced and what the file now
// contains; the base store holds the text behind both. The difference is not
// guessed at — it is recorded.
//
// What ilk gathers is evidence, not a verdict. Whether a local edit is a fix
// everyone needs or a quirk of one repository is a judgement, and the two
// sections that carry it are left for a person or an agent to write. A proposal
// that has not had them written cannot be submitted.
package contrib

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/basestore"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/manifest"
)

// Proposal is everything upstream needs to judge one repository's experience of
// a layer.
type Proposal struct {
	Layer        string                 `json:"layer"`
	LayerVersion string                 `json:"layer_version"`
	Repo         string                 `json:"repo"`
	Contribution *manifest.Contribution `json:"contribution,omitempty"`

	// Edits are the layer's own artifacts this repository changed.
	Edits []Edit `json:"edits,omitempty"`
	// Signals are the places the layer is fighting its adopter. They are the
	// half of the evidence a patch cannot carry: a check nobody can satisfy and
	// a default nobody keeps are both the layer being wrong, and neither shows
	// up as a diff.
	Signals []Signal `json:"signals,omitempty"`

	// Concerns are reasons not to send this upstream as it stands.
	Concerns []Concern `json:"concerns,omitempty"`
}

// Edit is one artifact that diverged from what the layer delivered.
type Edit struct {
	Path string `json:"path"`
	// Region is set when only a fenced block inside a shared file diverged.
	Region string `json:"region,omitempty"`
	// Source is the file in the layer's own tree this artifact came from, when
	// one can be identified.
	Source string `json:"source,omitempty"`
	// Delivered and Current are the two texts. Delivered is the layer's output;
	// Current is what the repository decided it should say instead.
	Delivered string `json:"-"`
	Current   string `json:"-"`
	Diff      string `json:"diff"`
	// Accepted marks a divergence somebody was asked about and kept. It is far
	// stronger evidence than an unexplained edit: a person saw ilk's version,
	// saw theirs, and chose theirs.
	Accepted bool `json:"accepted"`
	// Edits counts commits that have touched this path. One is a change of mind;
	// six is a repository repeatedly repairing the same thing.
	Edits int `json:"edits,omitempty"`
	// Survived counts commits since this path last changed. A divergence that has
	// outlived two hundred commits is load-bearing; one from this morning is a
	// work in progress, and upstream should be told which it is looking at.
	Survived int `json:"survived,omitempty"`
	// Portable records whether the layer's source file and its delivered content
	// are identical — that is, nothing was templated into it. Only then can the
	// diff be replayed upstream as-is. Otherwise it is evidence, and the change
	// has to be made properly by hand.
	Portable bool `json:"portable"`

	// prefix is the header a target prepended to the layer's content, dropped
	// from both sides of the patch because nobody upstream can change it.
	prefix string
}

// Concern is a reason a proposal should not be sent as it stands.
type Concern struct {
	Path   string `json:"path"`
	Line   int    `json:"line,omitempty"`
	Reason string `json:"reason"`
	// Blocking concerns stop submission outright. A repository name in a diff is
	// worth a second look; a credential in one is not a matter of opinion.
	Blocking bool `json:"blocking"`
}

// Find resolves the layer a proposal is about.
func Find(p *engine.Project, id string) (*engine.ResolvedLayer, error) {
	var matches []*engine.ResolvedLayer
	for _, l := range p.Layers {
		if id == "" || l.ID() == id || l.Name() == id {
			matches = append(matches, l)
		}
	}
	switch {
	case len(matches) == 0 && id == "":
		return nil, fmt.Errorf("no layers are adopted here — there is nothing to contribute back to")
	case len(matches) == 0:
		return nil, fmt.Errorf("no adopted layer called %q — %s", id, adoptedNames(p))
	case len(matches) > 1:
		return nil, fmt.Errorf("more than one layer matches %q — %s", id, adoptedNames(p))
	}
	return matches[0], nil
}

func adoptedNames(p *engine.Project) string {
	var names []string
	for _, l := range p.Layers {
		names = append(names, l.ID())
	}
	if len(names) == 0 {
		return "nothing is adopted"
	}
	sort.Strings(names)
	return "adopted: " + strings.Join(names, ", ")
}

// Build gathers everything this repository knows about a layer.
func Build(p *engine.Project, l *engine.ResolvedLayer) (*Proposal, error) {
	m := l.Loaded.Manifest
	prop := &Proposal{
		Layer:        l.ID(),
		LayerVersion: m.Version,
		Repo:         p.Repo.Name(),
		Contribution: m.Contribution,
	}

	locked := findLocked(p.Lock, l.ID())
	prop.Edits = collectEdits(p, l)
	prop.Signals = collectSignals(p, l, locked)
	prop.Concerns = screen(prop)

	return prop, nil
}

func findLocked(lk *lock.Lock, id string) *lock.Layer {
	for i := range lk.Layers {
		if lk.Layers[i].ID == id {
			return &lk.Layers[i]
		}
	}
	return nil
}

// Empty reports whether there is nothing to say. A repository that has used a
// layer exactly as shipped has learned something too — that the defaults were
// right — but that is not a pull request.
func (p *Proposal) Empty() bool { return len(p.Edits) == 0 && len(p.Signals) == 0 }

// Blocking returns the concerns that stop submission outright.
func (p *Proposal) Blocking() []Concern {
	var out []Concern
	for _, c := range p.Concerns {
		if c.Blocking {
			out = append(out, c)
		}
	}
	return out
}

// Portable reports whether every edit can be replayed upstream as a patch. When
// it is false the proposal still goes, but as evidence for a change somebody
// makes properly rather than as something to merge.
func (p *Proposal) Portable() bool {
	for _, e := range p.Edits {
		if !e.Portable {
			return false
		}
	}
	return len(p.Edits) > 0
}

// collectEdits finds every artifact of this layer that no longer matches what
// the layer produced.
//
// The comparison is against Delivered — the layer's own content — and not
// against Hash, which is what ilk expects to find. After a merge or an accept the
// two differ on purpose, and it is precisely that gap upstream wants to see:
// somebody was shown both versions and did not pick the layer's.
func collectEdits(p *engine.Project, l *engine.ResolvedLayer) []Edit {
	var out []Edit
	seenSkill := map[string]bool{}
	for _, f := range attributable(p, l) {
		if f.Mode == manifest.ModeCreateOnly {
			// create-only artifacts are seeds handed over on the first write. What
			// a repository did with its own file afterwards is not a comment on
			// the layer, and reporting it as one would bury the real signals.
			continue
		}
		// A skill is written once per agent target, from one source. Reporting each
		// copy would show the maintainer the same edit two or three times and
		// suggest a disagreement that is not there.
		if name, ok := skillName(l, f.Path); ok {
			if seenSkill[name] {
				continue
			}
			seenSkill[name] = true
		}
		delivered, ok := basestore.Get(p.Repo.Root, f.Delivered)
		if !ok {
			continue
		}
		current, ok := currentContent(p, f)
		if !ok {
			continue
		}
		if current == delivered {
			continue
		}

		e := Edit{
			Path:      f.Path,
			Region:    f.Region,
			Delivered: delivered,
			Current:   current,
			Accepted:  f.Hash != "" && f.Hash != f.Delivered,
		}
		e.Source, e.prefix, e.Portable = sourceFor(l, f, delivered)
		e.Diff = patchFor(e)
		e.Edits, e.Survived = history(p, f.Path)
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Region < out[j].Region
	})
	return out
}

// attributable lists the locked artifacts this layer is answerable for.
//
// Not the same as the artifacts it owns. A skill belongs to the layer that wrote
// it, but the file on disk is written by an agent target — the layer declares
// `skills:` and the target decides that Claude Code reads `.claude/skills/`. Going
// by ownership alone would make the single most valuable thing an adopter can
// improve, the wording of a skill, invisible to the layer that shipped it.
func attributable(p *engine.Project, l *engine.ResolvedLayer) []lock.File {
	var out []lock.File
	// The whole lockfile, not this layer's entry. A layer that contributes only
	// skills has no entry of its own — it writes no files directly — and going
	// via its entry would make exactly those layers uncontributable.
	for i := range p.Lock.Layers {
		for _, f := range p.Lock.Layers[i].Files {
			if f.Owner == l.ID() {
				out = append(out, f)
				continue
			}
			if _, ok := skillName(l, f.Path); ok {
				out = append(out, f)
			}
		}
	}
	return out
}

// skillName reports which of this layer's skills a path holds, if any.
func skillName(l *engine.ResolvedLayer, path string) (string, bool) {
	for _, s := range l.Loaded.Manifest.Skills {
		if strings.Contains(path, "/skills/"+s.Name+"/") {
			return s.Name, true
		}
	}
	return "", false
}

// patchFor renders the change as the diff upstream would actually apply.
//
// For a portable artifact that means a diff against the layer's own source path,
// not against where the file landed in this repository — a maintainer should not
// have to translate `.claude/skills/x/SKILL.md` back to `skills/x.md` in their
// head. Where a target wrapped the content in a generated header, the header is
// dropped from both sides: it is the target's output, regenerated every apply, and
// nobody upstream can change it.
func patchFor(e Edit) string {
	if !e.Portable || e.Source == "" {
		return unified(e.Path, e.Delivered, e.Current)
	}
	prefix := e.prefix
	if prefix != "" && !strings.HasPrefix(e.Current, prefix) {
		// The generated header was edited too. That is a target's business rather
		// than the layer's, so say so by showing the whole file as it stands.
		return unified(e.Path, e.Delivered, e.Current)
	}
	return unified(e.Source,
		strings.TrimPrefix(e.Delivered, prefix),
		strings.TrimPrefix(e.Current, prefix))
}

// currentContent reads what the artifact says now: the whole file for managed
// mode, the fenced body for a region inside a file somebody else owns.
func currentContent(p *engine.Project, f lock.File) (string, bool) {
	data, err := os.ReadFile(p.Repo.Path(f.Path))
	if err != nil {
		return "", false
	}
	if f.Region == "" {
		return string(data), true
	}
	return extractRegion(f.Path, string(data), f.Owner, f.Region)
}
