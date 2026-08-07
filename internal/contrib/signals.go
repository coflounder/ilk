package contrib

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/checks"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/fence"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/manifest"
)

// Signal is one place a layer is fighting the repository that adopted it.
//
// These are the findings a patch cannot carry. A default nobody keeps, a check
// nobody can satisfy, and an exemption that never shrinks are all the layer
// being wrong about something, and none of them appears as a diff. Upstream
// receiving only patches sees the repairs and never the reasons.
type Signal struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Detail  string `json:"detail"`
	// Weight orders the report. It is a rough measure of how much the repository
	// has invested in disagreeing, not a score anybody should tune against.
	Weight int `json:"weight"`
}

// Signal kinds.
const (
	// SignalVariable is a default the repository chose not to keep.
	SignalVariable = "variable"
	// SignalCheck is a check of this layer that fails right now. A check failing
	// in a repository that has otherwise made the layer work is usually the
	// layer asking for something it never explained how to provide.
	SignalCheck = "failing-check"
	// SignalBaseline is work the layer exempted at adoption and nobody ever
	// brought into conformance.
	SignalBaseline = "baseline"
	// SignalUnadopted is an artifact the layer delivers that the repository
	// deleted. Deleting is a stronger opinion than editing.
	SignalUnadopted = "deleted"
)

// collectSignals reads friction out of state ilk already keeps.
//
// Nothing here runs a survey or asks anybody a question. Every signal is derived
// from the lockfile, the config, git history and a check run — which is the point:
// the evidence is a by-product of using the layer, so contributing costs the
// adopter nothing but the judgement.
func collectSignals(p *engine.Project, l *engine.ResolvedLayer, locked *lock.Layer) []Signal {
	var out []Signal
	out = append(out, variableSignals(l)...)
	out = append(out, checkSignals(p, l)...)
	if locked != nil {
		out = append(out, baselineSignals(p, l, locked)...)
		out = append(out, deletionSignals(p, l, locked)...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Weight > out[j].Weight })
	return out
}

// variableSignals reports defaults this repository overrode.
//
// One repository changing a default is a preference. Upstream seeing the same
// override in proposal after proposal is a default that is simply wrong, and that
// pattern is only visible if each one is reported.
func variableSignals(l *engine.ResolvedLayer) []Signal {
	var out []Signal
	for name, v := range l.Loaded.Manifest.Variables {
		chosen, ok := l.Vars[name]
		if !ok || chosen == v.Default {
			continue
		}
		if v.Default == "" {
			// A variable with no default is one the layer requires the adopter to
			// supply. Setting it is using the layer, not disagreeing with it.
			continue
		}
		out = append(out, Signal{
			Kind:    SignalVariable,
			Subject: name,
			Detail:  fmt.Sprintf("the layer defaults to %q; this repository uses %q", v.Default, chosen),
			Weight:  2,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Subject < out[j].Subject })
	return out
}

// checkSignals runs this layer's checks and reports the ones that fail.
//
// A failing check in a repository that has otherwise made the layer work is
// evidence about the check, not only about the repository. Upstream should see
// what the finding actually said, because "this check fails everywhere" and "this
// check fails here for a good reason" read very differently once you can see it.
func checkSignals(p *engine.Project, l *engine.ResolvedLayer) []Signal {
	var ids []string
	for _, c := range l.Loaded.Manifest.Checks {
		ids = append(ids, c.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	report, err := checks.Run(p, checks.Options{Only: ids})
	if err != nil {
		return nil
	}

	var out []Signal
	for _, r := range report.Results {
		switch r.Status {
		case checks.StatusFail:
			detail := fmt.Sprintf("%d finding(s)", len(r.Findings))
			if len(r.Findings) > 0 {
				detail += ": " + r.Findings[0].Message
				if len(r.Findings) > 1 {
					detail += fmt.Sprintf(" (and %d more)", len(r.Findings)-1)
				}
			}
			out = append(out, Signal{Kind: SignalCheck, Subject: r.ID, Detail: detail, Weight: 5})
		case checks.StatusError:
			// A check that cannot run is worse than one that fails: it reports
			// nothing at all, and a repository can sit under it for months
			// believing it is covered.
			out = append(out, Signal{
				Kind: SignalCheck, Subject: r.ID,
				Detail: "cannot run here: " + r.Reason, Weight: 6,
			})
		}
	}
	return out
}

// baselineSignals reports exemptions granted at adoption that nobody cleared.
//
// The baseline is deliberate — a layer governs what happens next, not what came
// before — but a baseline that never shrinks means the layer's demands do not
// match how this repository actually works. That is worth upstream knowing, and
// it is invisible from any single diff.
func baselineSignals(p *engine.Project, l *engine.ResolvedLayer, locked *lock.Layer) []Signal {
	if len(locked.Baseline) == 0 {
		return nil
	}
	remaining := 0
	for _, path := range locked.Baseline {
		if _, err := os.Stat(p.Repo.Path(path)); err == nil {
			remaining++
		}
	}
	if remaining == 0 {
		return nil
	}
	detail := fmt.Sprintf("%d of %d file(s) exempted when the layer was adopted are still exempt", remaining, len(locked.Baseline))
	if remaining == len(locked.Baseline) {
		detail += " — none has been brought into conformance"
	}
	return []Signal{{
		Kind: SignalBaseline, Subject: l.ID(), Detail: detail,
		Weight: 1 + remaining/10,
	}}
}

// deletionSignals reports artifacts the layer delivers that are simply gone.
//
// Deleting is a stronger opinion than editing: somebody decided the layer was
// wrong to ship this at all. ilk does not recreate it, so without this the
// decision stays inside one repository for ever.
func deletionSignals(p *engine.Project, l *engine.ResolvedLayer, locked *lock.Layer) []Signal {
	var out []Signal
	for _, f := range locked.Files {
		if f.Owner != l.ID() || f.Mode == manifest.ModeCreateOnly || f.Region != "" {
			continue
		}
		if _, err := os.Stat(p.Repo.Path(f.Path)); err == nil {
			continue
		}
		out = append(out, Signal{
			Kind: SignalUnadopted, Subject: f.Path,
			Detail: "delivered by the layer and deleted here", Weight: 4,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Subject < out[j].Subject })
	return out
}

// history measures how much a path has been argued with.
//
// Two numbers, both from git, because they mean different things and reporting
// only one misleads. Edits counts how often the repository has come back to this
// file: once is a change of mind, six times is a repository repeatedly repairing
// the same thing. Survived counts commits since it last changed, which separates
// a divergence that has proved itself from one somebody is still in the middle of.
func history(p *engine.Project, path string) (edits, survived int) {
	if !p.Repo.IsGit() {
		return 0, 0
	}
	if commits, ok := p.Repo.CommitsTouching([]string{path}, 0, 50); ok {
		edits = len(commits)
	}
	if sha, ok := p.Repo.LastCommitSHA(path); ok && sha != "" {
		if commits, ok := p.Repo.CommitsInRange(sha, nil, 0); ok {
			survived = len(commits)
		}
	}
	return edits, survived
}

// extractRegion reads a fenced block out of a file somebody else owns.
func extractRegion(path, content, owner, region string) (string, bool) {
	body, found, err := fence.Extract(content, fence.StyleFor(path), fence.Marker{Layer: owner, Region: region})
	if err != nil || !found {
		return "", false
	}
	return body, true
}

// sourceFor identifies the file in the layer's own tree an artifact came from,
// and whether the diff can be replayed there directly.
//
// Portability is the whole question. If the layer's source survives verbatim in
// what was delivered — either as the whole file, or as everything after a header
// a target generated — then nothing was templated in, and a patch applies upstream
// unchanged. If it does not survive verbatim, the delivered text contains this
// repository's values, and a patch built from it would carry them upstream too.
// ilk will not guess its way back through a template: it says so, and the change
// gets made by hand.
func sourceFor(l *engine.ResolvedLayer, f lock.File, delivered string) (source, prefix string, portable bool) {
	m := l.Loaded.Manifest
	var candidate string
	switch {
	case f.Region != "":
		// Every instruction a layer declares is projected into one region, so a
		// layer with two of them has no 1:1 mapping back to a source file. Rather
		// than pick, ilk reports the region as unportable and lets the change be
		// made where it belongs.
		if len(m.Instructions) == 1 {
			candidate = m.Instructions[0].Src
		}
	default:
		for _, file := range m.Files {
			dest, err := renderPath(l, file.Dest)
			if err == nil && dest == f.Path {
				candidate = file.Src
			}
		}
		for _, sk := range m.Skills {
			if strings.Contains(f.Path, "/skills/"+sk.Name+"/") {
				candidate = sk.Src
			}
		}
	}
	if candidate == "" {
		return "", "", false
	}
	text, err := readLayerFile(l, candidate)
	if err != nil {
		return candidate, "", false
	}

	src := strings.TrimRight(text, "\n")
	del := strings.TrimRight(delivered, "\n")
	switch {
	case del == src:
		return candidate, "", true
	case src != "" && strings.HasSuffix(del, "\n"+src):
		// A target prepended a generated header — the frontmatter a skill-aware
		// agent expects. Everything after it is the layer's source verbatim, so
		// the artifact is still portable; the header simply is not part of it.
		return candidate, delivered[:len(del)-len(src)], true
	}
	return candidate, "", false
}
