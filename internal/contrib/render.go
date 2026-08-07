package contrib

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Marker is what a proposal says where only a person can write.
//
// ilk gathers evidence; it cannot say whether an edit is a fix everybody needs
// or a quirk of one repository, and a generated paragraph guessing at that is
// worse than a blank one — it reads like an argument and carries none. So the two
// sections that matter are left explicitly unwritten, and submission refuses
// while a marker survives.
const Marker = "TODO(you):"

// Dir is where proposals are drafted, inside ilk's own space.
//
// Not in the record: the record's checks — naming, frontmatter, links, staleness —
// would all then apply to a document that is a message rather than a claim about
// this project, and the record layer's grammar would have to grow a case for
// something that is on its way out of the repository.
const Dir = ".ilk/proposals"

// Path returns where a proposal for a layer is drafted.
func (p *Proposal) Path() string {
	return filepath.Join(Dir, strings.ReplaceAll(p.Layer, "/", "-")+".md")
}

// Title is the one-line summary a pull request carries.
func (p *Proposal) Title() string {
	return fmt.Sprintf("%s: what %s learned", p.Layer, p.Repo)
}

// Render writes the proposal as the document a maintainer will read.
//
// The order is deliberate. Judgement first, because that is what upstream is being
// asked for and burying it under a diff invites the reviewer to skim. Evidence
// second, machine-gathered and uneditorialised. The patch last, because it is the
// least interesting part: the change is easy once the case for it is made.
func (p *Proposal) Render() string {
	var b strings.Builder

	b.WriteString("---\n")
	fmt.Fprintf(&b, "layer: %s\n", p.Layer)
	fmt.Fprintf(&b, "layer_version: %s\n", p.LayerVersion)
	fmt.Fprintf(&b, "from: %s\n", p.Repo)
	fmt.Fprintf(&b, "status: draft\n")
	fmt.Fprintf(&b, "portable: %t\n", p.Portable())
	b.WriteString("---\n\n")

	fmt.Fprintf(&b, "# %s\n\n", p.Title())
	fmt.Fprintf(&b, "A repository using `%s` %s found these. Everything under **Evidence** was\n",
		p.Layer, versionPhrase(p.LayerVersion))
	b.WriteString("gathered by `ilk contribute` from what ilk recorded; the two sections above it are\n")
	b.WriteString("judgement, and no tool can supply them.\n\n")

	b.WriteString("## What this repository needed\n\n")
	b.WriteString(Marker + " what were you actually trying to do, and what did the layer do instead?\n")
	b.WriteString("Describe the situation, not the patch — the patch is below and a maintainer can read it.\n\n")

	b.WriteString("## Why this is not specific to this repository\n\n")
	b.WriteString(Marker + " the load-bearing section. Why would another repository hit this?\n")
	b.WriteString("If the honest answer is that it would not, say so and send it anyway as a signal —\n")
	b.WriteString("a layer wrong for one adopter in a way it can name is more useful than silence.\n\n")

	b.WriteString("## Evidence\n\n")
	p.renderEvidence(&b)

	if len(p.Edits) > 0 {
		b.WriteString("## Proposed change\n\n")
		p.renderPatches(&b)
	}

	if len(p.Concerns) > 0 {
		b.WriteString("## Before sending\n\n")
		for _, c := range p.Concerns {
			marker := "-"
			if c.Blocking {
				marker = "- **blocking**"
			}
			fmt.Fprintf(&b, "%s `%s`", marker, c.Path)
			if c.Line > 0 {
				fmt.Fprintf(&b, " line %d", c.Line)
			}
			fmt.Fprintf(&b, " — %s\n", c.Reason)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func versionPhrase(v string) string {
	if v == "" {
		return "in production"
	}
	return "at version " + v
}

func (p *Proposal) renderEvidence(b *strings.Builder) {
	if len(p.Edits) == 0 && len(p.Signals) == 0 {
		b.WriteString("None gathered. This layer is being used exactly as it shipped.\n\n")
		return
	}

	if len(p.Edits) > 0 {
		b.WriteString("### What was changed after the layer wrote it\n\n")
		b.WriteString("| artifact | history | agreed | replayable |\n")
		b.WriteString("|---|---|---|---|\n")
		for _, e := range p.Edits {
			fmt.Fprintf(b, "| `%s` | %s | %s | %s |\n",
				e.label(), e.HistoryPhrase(), yesNo(e.Accepted), yesNo(e.Portable))
		}
		b.WriteString("\n")
		b.WriteString("*agreed* means somebody was shown both versions and kept this one — ilk asked, and\n")
		b.WriteString("the answer was not the layer's. *replayable* means nothing was templated into the\n")
		b.WriteString("file, so the patch below applies to the layer's own source unchanged.\n\n")
	}

	if len(p.Signals) > 0 {
		b.WriteString("### Where the layer is fighting this repository\n\n")
		b.WriteString("None of these is a patch. They are the places the layer asked for something the\n")
		b.WriteString("repository could not give, or offered a default it did not keep.\n\n")
		for _, s := range p.Signals {
			fmt.Fprintf(b, "- **%s** `%s` — %s\n", s.Kind, s.Subject, s.Detail)
		}
		b.WriteString("\n")
	}
}

func (p *Proposal) renderPatches(b *strings.Builder) {
	if !p.Portable() {
		b.WriteString("Some of this cannot be replayed upstream directly: the artifacts below were\n")
		b.WriteString("rendered with this repository's values, so a patch built from them would carry\n")
		b.WriteString("those values with it. Read them as evidence and make the change in the layer's\n")
		b.WriteString("source by hand.\n\n")
	}
	for _, e := range p.Edits {
		fmt.Fprintf(b, "### `%s`\n\n", e.label())
		if e.Source != "" {
			fmt.Fprintf(b, "Layer source: `%s`", e.Source)
			if !e.Portable {
				b.WriteString(" — **rendered, not verbatim**")
			}
			b.WriteString("\n\n")
		}
		b.WriteString("```diff\n")
		b.WriteString(strings.TrimRight(e.Diff, "\n"))
		b.WriteString("\n```\n\n")
	}
}

func (e Edit) label() string {
	if e.Region != "" {
		return e.Path + " [" + e.Region + "]"
	}
	return e.Path
}

// HistoryPhrase says how much the repository has argued with this artifact,
// in words rather than two bare numbers nobody can interpret.
func (e Edit) HistoryPhrase() string {
	switch {
	case e.Edits == 0:
		return "uncommitted"
	case e.Edits == 1 && e.Survived == 0:
		return "changed once, just now"
	case e.Edits == 1:
		return fmt.Sprintf("changed once, held for %d commits", e.Survived)
	default:
		return fmt.Sprintf("changed %d times, held for %d commits", e.Edits, e.Survived)
	}
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

// unwritten reports the judgement sections nobody has filled in yet.
//
// This is the gate. A proposal whose case has not been made is not a contribution;
// it is a diff with a covering note, and a maintainer receiving a stream of them
// learns to ignore the whole channel.
var markerLine = regexp.MustCompile(`(?m)^.*` + regexp.QuoteMeta(Marker) + `.*$`)

func unwritten(document string) []string {
	var out []string
	for _, m := range markerLine.FindAllString(document, -1) {
		out = append(out, strings.TrimSpace(m))
	}
	return out
}
