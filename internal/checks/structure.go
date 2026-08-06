package checks

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/engine"
)

// Two checks that let a layer enforce structure across a set of documents rather
// than inside one.
//
// The MetaHarness idea they exist for is that a plan is one connected blueprint,
// not a pile of loosely related entries: every ticket belongs to a checkpoint,
// every commitment has acceptance criteria. Both of those are shapes no
// single-document check can see, and both are general enough that a layer should
// not have to ship a parser to assert them.

// ---------------------------------------------------------------- references

// checkReferences verifies that a frontmatter field naming another document
// resolves to a document that exists.
//
// A plan pointing at a milestone nobody ever wrote is the drift this catches —
// the error the essay describes as "PS-portal ticket 6 points at milestone M3,
// which does not exist; relink or remove".
func checkReferences(p *engine.Project, args map[string]any) ([]Finding, error) {
	dirs := stringSlice(args["dirs"])
	field := asString(args["field"])
	targetDirs := stringSlice(args["targets"])
	required := asString(args["required"]) == "true"
	allow := stringSet(args["allow"])
	exempt := stringSet(args["exempt"])

	if field == "" {
		return nil, fmt.Errorf("builtin.references needs a `field:` naming the frontmatter key to resolve")
	}
	if len(targetDirs) == 0 {
		targetDirs = dirs
	}

	known, err := documentIDs(p, targetDirs)
	if err != nil {
		return nil, err
	}

	match, err := compileMatch(args["match"])
	if err != nil {
		return nil, err
	}

	baseline := p.Baseline()
	var findings []Finding
	for _, dir := range dirs {
		files, err := markdownFiles(p.Repo.Path(dir))
		if err != nil {
			continue
		}
		for _, path := range files {
			rel := repoRel(p.Repo.Root, path)
			base := filepath.Base(path)
			if exempt[base] || baseline[rel] || !match(base) {
				continue
			}
			meta, err := readFrontmatter(path)
			if err != nil {
				continue
			}

			raw, present := meta[field]
			values := stringSlice(raw)
			if !present || len(values) == 0 {
				if required {
					findings = append(findings, Finding{
						Path:    rel,
						Line:    1,
						Message: fmt.Sprintf("no `%s:` in the frontmatter, so this document belongs to nothing", field),
					})
				}
				continue
			}
			for _, value := range values {
				value = strings.TrimSpace(value)
				if value == "" || allow[value] {
					// An explicit opt-out is a decision somebody made; a missing
					// field is an oversight. Only the second is a finding.
					continue
				}
				if !known[value] {
					findings = append(findings, Finding{
						Path:    rel,
						Line:    1,
						Message: fmt.Sprintf("`%s: %s` names a document that does not exist in %s", field, value, strings.Join(targetDirs, ", ")),
					})
				}
			}
		}
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].Path < findings[j].Path })
	return findings, nil
}

// documentIDs collects the `id:` of every document in the given directories.
func documentIDs(p *engine.Project, dirs []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, dir := range dirs {
		files, err := markdownFiles(p.Repo.Path(dir))
		if err != nil {
			continue
		}
		for _, path := range files {
			meta, err := readFrontmatter(path)
			if err != nil {
				continue
			}
			if id := strings.TrimSpace(asString(meta["id"])); id != "" {
				out[id] = true
			}
		}
	}
	return out, nil
}

// ------------------------------------------------------------------- section

// checkSection verifies that a document contains a named heading with content
// under it.
//
// It exists for the acceptance-criteria rule: a commitment with no statement of
// what "done" means is a claim nobody can evaluate, and the point of writing it
// down is that pass and fail become legible to somebody other than its author.
func checkSection(p *engine.Project, args map[string]any) ([]Finding, error) {
	dirs := stringSlice(args["dirs"])
	heading := asString(args["heading"])
	minItems := intArg(args["min_items"], 1)
	exempt := stringSet(args["exempt"])

	if heading == "" {
		return nil, fmt.Errorf("builtin.section needs a `heading:` to look for")
	}
	match, err := compileMatch(args["match"])
	if err != nil {
		return nil, err
	}

	headingPattern := regexp.MustCompile(`(?i)^#{1,6}\s+` + regexp.QuoteMeta(heading) + `\s*$`)
	itemPattern := regexp.MustCompile(`^\s*([-*+]|\d+\.)\s+\S`)

	baseline := p.Baseline()
	var findings []Finding
	for _, dir := range dirs {
		files, err := markdownFiles(p.Repo.Path(dir))
		if err != nil {
			continue
		}
		for _, path := range files {
			rel := repoRel(p.Repo.Root, path)
			base := filepath.Base(path)
			if exempt[base] || baseline[rel] || !match(base) {
				continue
			}
			content, err := readFile(path)
			if err != nil {
				continue
			}

			lines := strings.Split(content, "\n")
			start := -1
			for i, line := range lines {
				if headingPattern.MatchString(line) {
					start = i
					break
				}
			}
			if start < 0 {
				findings = append(findings, Finding{
					Path:    rel,
					Message: fmt.Sprintf("no `%s` section", heading),
				})
				continue
			}

			items := 0
			for _, line := range lines[start+1:] {
				if strings.HasPrefix(line, "#") {
					break // the next heading ends the section
				}
				if itemPattern.MatchString(line) {
					items++
				}
			}
			if items < minItems {
				noun := "entries"
				if minItems == 1 {
					noun = "entry"
				}
				findings = append(findings, Finding{
					Path:    rel,
					Line:    start + 1,
					Message: fmt.Sprintf("the `%s` section has %d of the %d %s it needs", heading, items, minItems, noun),
				})
			}
		}
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].Path < findings[j].Path })
	return findings, nil
}

// compileMatch builds a filename filter. An absent pattern matches everything,
// which is what a layer wanting to check a whole directory expects.
func compileMatch(v any) (func(string) bool, error) {
	pattern := strings.TrimSpace(asString(v))
	if pattern == "" {
		return func(string) bool { return true }, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("match: %w", err)
	}
	return re.MatchString, nil
}
