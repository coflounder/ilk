package checks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/fence"
	"github.com/coflounder/ilk/internal/lock"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/coflounder/ilk/internal/merge"
	"github.com/coflounder/ilk/internal/repo"
	"gopkg.in/yaml.v3"
)

// builtinFunc is a check implemented in Go rather than as a shell command.
type builtinFunc func(p *engine.Project, args map[string]any) ([]Finding, error)

var builtins = map[string]builtinFunc{
	"builtin.frontmatter": checkFrontmatter,
	"builtin.naming":      checkNaming,
	"builtin.links":       checkLinks,
	"builtin.stale":       checkStale,
	"builtin.coverage":    checkCoverage,
	"builtin.drift":       checkDrift,
	"builtin.budget":      checkBudget,
	"builtin.conflicts":   checkConflicts,
}

func builtinNames() []string {
	names := make([]string, 0, len(builtins))
	for k := range builtins {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// coreChecks are registered in every repository regardless of adopted layers.
// They validate ilk's own contract rather than any layer's.
func coreChecks() []manifest.Check {
	return []manifest.Check{
		{
			ID:    "ilk.drift",
			Kind:  "builtin.drift",
			Title: "Generated files match what ilk would write",
			Fix:   "Keep your version with `ilk apply --accept`, or restore ilk's with `ilk apply --force`. If the edit belongs to you rather than the layer, move it outside the ilk:begin/ilk:end markers first.",
		},
		{
			ID:    "ilk.conflicts",
			Kind:  "builtin.conflicts",
			Title: "No unresolved merge conflicts are left in ilk's files",
			Fix:   "Open the file, pick the version you want, and delete the <<<<<<< / ======= / >>>>>>> lines. Then run `ilk apply --accept` so ilk records your resolution as the new baseline.",
		},
		{
			ID:    "ilk.budget",
			Kind:  "builtin.budget",
			Title: "Always-on agent instructions stay within budget",
			Fix:   "Move detail out of a layer's `instructions:` and into a `skills:` file, which loads only when its situation applies. Raise the ceiling under `budget:` in .ilk/config.yaml only if you have measured that the extra context earns its place.",
		},
	}
}

// ---------------------------------------------------------------- frontmatter

func checkFrontmatter(p *engine.Project, args map[string]any) ([]Finding, error) {
	dirs := stringSlice(args["dirs"])
	required := stringSlice(args["require"])
	exempt := stringSet(args["exempt"])

	baseline := p.Baseline()

	var findings []Finding
	for _, dir := range dirs {
		files, err := markdownFiles(p.Repo.Path(dir))
		if err != nil {
			continue // A directory a layer declares but nobody created yet is not a failure.
		}
		for _, path := range files {
			rel := repoRel(p.Repo.Root, path)
			if exempt[filepath.Base(path)] || baseline[rel] {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			meta, err := parseFrontmatterStrict(string(data))
			if err != nil {
				findings = append(findings, Finding{
					Path:    rel,
					Line:    1,
					Message: fmt.Sprintf("%s; needs %s", err, strings.Join(required, ", ")),
				})
				continue
			}
			var missing []string
			for _, key := range required {
				if v, ok := meta[key]; !ok || isBlank(v) {
					missing = append(missing, key)
				}
			}
			if len(missing) > 0 {
				findings = append(findings, Finding{
					Path:    rel,
					Line:    1,
					Message: "frontmatter is missing " + strings.Join(missing, ", "),
				})
			}
		}
	}
	return findings, nil
}

// --------------------------------------------------------------------- naming

func checkNaming(p *engine.Project, args map[string]any) ([]Finding, error) {
	exempt := stringSet(args["exempt"])
	rules, _ := args["rules"].([]any)
	baseline := p.Baseline()

	var findings []Finding
	for _, raw := range rules {
		rule, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		dir := asString(rule["dir"])
		patternText := asString(rule["pattern"])
		example := asString(rule["example"])
		pattern, err := regexp.Compile(patternText)
		if err != nil {
			return nil, fmt.Errorf("naming rule for %s: %w", dir, err)
		}
		files, err := markdownFiles(p.Repo.Path(dir))
		if err != nil {
			continue
		}
		for _, path := range files {
			base := filepath.Base(path)
			rel := repoRel(p.Repo.Root, path)
			if exempt[base] || baseline[rel] || pattern.MatchString(base) {
				continue
			}
			findings = append(findings, Finding{
				Path:    rel,
				Message: fmt.Sprintf("filename does not match the grammar for %s/ (e.g. %s)", dir, example),
			})
		}
	}
	return findings, nil
}

// ---------------------------------------------------------------------- links

var markdownLink = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)

func checkLinks(p *engine.Project, args map[string]any) ([]Finding, error) {
	dirs := stringSlice(args["dirs"])
	baseline := p.Baseline()

	var findings []Finding
	for _, dir := range dirs {
		files, err := markdownFiles(p.Repo.Path(dir))
		if err != nil {
			continue
		}
		for _, path := range files {
			rel := repoRel(p.Repo.Root, path)
			if baseline[rel] {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			for i, line := range strings.Split(string(data), "\n") {
				for _, m := range markdownLink.FindAllStringSubmatch(line, -1) {
					target := m[1]
					if isExternalLink(target) {
						continue
					}
					target = strings.SplitN(target, "#", 2)[0]
					if target == "" {
						continue
					}
					resolved := filepath.Join(filepath.Dir(path), filepath.FromSlash(target))
					if _, err := os.Stat(resolved); err != nil {
						findings = append(findings, Finding{
							Path:    rel,
							Line:    i + 1,
							Message: fmt.Sprintf("link target %q does not exist", m[1]),
						})
					}
				}
			}
		}
	}
	return findings, nil
}

func isExternalLink(target string) bool {
	lower := strings.ToLower(target)
	for _, prefix := range []string{"http://", "https://", "mailto:", "tel:", "ftp://", "//"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return strings.HasPrefix(target, "#")
}

// ---------------------------------------------------------------------- stale

// checkStale reports documents whose subject has moved on without them.
//
// Staleness is measured against **what the document says it covers**, not against
// the calendar. A fixed expiry cannot work across projects of different maturity:
// a week is an eternity for a prototype and nothing at all for a stable subsystem,
// and any universal number is wrong for almost everybody. Worse, a timer teaches
// people to bump a date to silence it, which is the one thing you never want a
// documentation check to train.
//
// Coupling self-calibrates. Code nobody has touched cannot make its documentation
// stale, however old that documentation is; code rewritten yesterday makes it stale
// immediately. The project's own pace sets the pace of review.
func checkStale(p *engine.Project, args map[string]any) ([]Finding, error) {
	dirs := stringSlice(args["dirs"])
	exempt := stringSet(args["exempt"])
	baseline := p.Baseline()

	threshold := intArg(args["review_after_commits"], 10)
	// An absolute age limit is off unless a project asks for it. It is the escape
	// hatch for documents that go stale because the world moved rather than the
	// code — an external API, a vendor's behaviour — which no amount of watching
	// this repository will ever detect.
	maxAgeDays := intArg(args["max_age_days"], 0)

	if !p.Repo.IsGit() {
		// Without history there is nothing to compare against, and a guess would
		// be worse than silence.
		return nil, nil
	}

	var findings []Finding
	for _, dir := range dirs {
		files, err := markdownFiles(p.Repo.Path(dir))
		if err != nil {
			continue
		}
		for _, path := range files {
			rel := repoRel(p.Repo.Root, path)
			if exempt[filepath.Base(path)] || baseline[rel] {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			meta, err := parseFrontmatterStrict(string(data))
			if err != nil {
				continue // the frontmatter check's problem, not this one
			}

			covers := stringSlice(meta["covers"])
			if len(covers) == 0 {
				continue // record.coverage reports this; saying it twice helps nobody
			}

			perDoc := intArgOr(meta["review_after_commits"], threshold)
			if finding, stale := staleAgainstCoverage(p, rel, covers, meta, perDoc); stale {
				findings = append(findings, finding)
				continue
			}

			// The absolute backstop deliberately trusts only the declared
			// `updated:`. A commit touching the document proves somebody changed
			// it — a reformat, a typo fix — not that anybody read it, and this
			// check is asking how long since a person last confirmed it.
			if maxAgeDays > 0 {
				if declared, ok := parseDate(asString(meta["updated"])); ok {
					if age := int(time.Since(declared).Hours() / 24); age > maxAgeDays {
						findings = append(findings, Finding{
							Path:    rel,
							Message: fmt.Sprintf("last reviewed %d days ago; this project reviews documents at least every %d days regardless of code changes", age, maxAgeDays),
						})
					}
				}
			}
		}
	}
	return findings, nil
}

// changesSince asks git what has happened to the covered paths since the document
// was last confirmed, and returns the commits that count.
//
// Which signal marks "last confirmed" matters. The document's own last commit is
// the honest one — it cannot be moved without committing something — and a commit
// range measured from it is exact, where a timestamp is not: several commits can
// share a second, and `--since` would then count the very commit being measured
// from. But a `updated:` dated later than that commit means somebody has just
// reviewed the document and not committed yet, so a date wins when it is newer.
func changesSince(p *engine.Project, rel string, covers []string, meta map[string]any, limit int) ([]repo.Commit, bool) {
	docCommit, hasCommit := p.Repo.LastCommitSHA(rel)
	docTime, _ := p.Repo.LastCommitTime(rel)

	if declared, ok := parseDate(asString(meta["updated"])); ok {
		// A date means a day, so anything committed that day is accounted for.
		endOfDay := declared.Add(24*time.Hour - time.Second).Unix()
		if !hasCommit || endOfDay > docTime {
			return p.Repo.CommitsTouching(covers, endOfDay, limit)
		}
	}
	if !hasCommit {
		return nil, false
	}
	return p.Repo.CommitsInRange(docCommit, covers, limit)
}

// staleAgainstCoverage reports a document whose covered paths have moved on
// further than the project tolerates.
func staleAgainstCoverage(p *engine.Project, rel string, covers []string, meta map[string]any, threshold int) (Finding, bool) {
	// Ask for one more than the threshold: enough to know the limit is crossed,
	// without walking the whole history of a busy subsystem.
	commits, ok := changesSince(p, rel, covers, meta, threshold+1)
	if !ok || len(commits) <= threshold {
		return Finding{}, false
	}

	newest := commits[0]
	reviewed := "an unknown date"
	if declared, ok := parseDate(asString(meta["updated"])); ok {
		reviewed = declared.Format("2006-01-02")
	}
	return Finding{
		Path: rel,
		Message: fmt.Sprintf(
			"%d+ commits touched %s since this was last reviewed (%s); most recent: %s %q, %s ago",
			len(commits), strings.Join(covers, ", "), reviewed,
			newest.SHA[:7], newest.Subject, humanDuration(time.Since(time.Unix(newest.When, 0)))),
	}, true
}

// humanDuration renders an age the way somebody would say it out loud.
func humanDuration(d time.Duration) string {
	switch days := int(d.Hours() / 24); {
	case days < 1:
		return "under a day"
	case days == 1:
		return "1 day"
	case days < 60:
		return fmt.Sprintf("%d days", days)
	default:
		return fmt.Sprintf("%d months", days/30)
	}
}

// ------------------------------------------------------------------- coverage

// checkCoverage reports documents that cannot be checked for staleness at all.
//
// A document with no `covers:` is never stale, which sounds harmless and is not:
// it is indistinguishable from a document that is always accurate. So is one whose
// patterns match nothing — a typo in a glob silently exempts a document for ever,
// which is the worst outcome of the three because it looks like it is working.
func checkCoverage(p *engine.Project, args map[string]any) ([]Finding, error) {
	dirs := stringSlice(args["dirs"])
	exempt := stringSet(args["exempt"])
	baseline := p.Baseline()

	var findings []Finding
	for _, dir := range dirs {
		files, err := markdownFiles(p.Repo.Path(dir))
		if err != nil {
			continue
		}
		for _, path := range files {
			rel := repoRel(p.Repo.Root, path)
			if exempt[filepath.Base(path)] || baseline[rel] {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			meta, err := parseFrontmatterStrict(string(data))
			if err != nil {
				continue
			}

			raw, declared := meta["covers"]
			covers := stringSlice(raw)
			if !declared {
				findings = append(findings, Finding{
					Path:    rel,
					Line:    1,
					Message: "declares no `covers:`, so nothing can ever mark it stale",
				})
				continue
			}
			if len(covers) == 0 {
				// An explicit empty list is a decision, not an oversight: some
				// documents genuinely are not coupled to any path — a rationale,
				// a glossary, a convention. Turning that into a declaration is
				// the point; leaving it implicit is what this check objects to.
				continue
			}
			for _, pattern := range covers {
				if !p.Repo.MatchesAnything(pattern) {
					findings = append(findings, Finding{
						Path:    rel,
						Line:    1,
						Message: fmt.Sprintf("`covers: %s` matches no tracked file, so this document is exempt from staleness by accident", pattern),
					})
				}
			}
		}
	}
	return findings, nil
}

func intArg(v any, fallback int) int {
	if s := strings.TrimSpace(asString(v)); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return fallback
}

// intArgOr lets a single document override the project default.
func intArgOr(v any, fallback int) int { return intArg(v, fallback) }

// ---------------------------------------------------------------------- drift

// checkDrift compares what is on disk against what ilk recorded writing. It is
// how a repository notices that someone edited inside a generated block, or that
// a generated file was deleted.
func checkDrift(p *engine.Project, _ map[string]any) ([]Finding, error) {
	var findings []Finding
	// A file holding conflict markers is reported by ilk.conflicts, which says
	// something far more useful than "edited since ilk wrote it".
	hasMarkers := func(s string) bool {
		for _, line := range strings.Split(s, "\n") {
			if strings.HasPrefix(line, merge.ConflictMarker) {
				return true
			}
		}
		return false
	}
	for _, entry := range p.Lock.Layers {
		for _, f := range entry.Files {
			if f.Mode == manifest.ModeCreateOnly || f.Mode == engine.ModeDir || f.Hash == "" {
				continue
			}
			data, err := os.ReadFile(p.Repo.Path(f.Path))
			if os.IsNotExist(err) {
				findings = append(findings, Finding{
					Path:    f.Path,
					Message: fmt.Sprintf("missing; %s expects to own it", entry.ID),
				})
				continue
			}
			if err != nil {
				return nil, err
			}
			if hasMarkers(string(data)) {
				continue
			}
			switch f.Mode {
			case manifest.ModeManaged, manifest.ModeMerge:
				if lock.Hash(string(data)) != f.Hash {
					findings = append(findings, Finding{
						Path:    f.Path,
						Message: fmt.Sprintf("edited since ilk wrote it (owner %s)", entry.ID),
					})
				}
			case manifest.ModeRegion, manifest.ModeAppendOnce:
				body, present, err := fence.Extract(string(data), fence.StyleFor(f.Path), fence.Marker{Layer: entry.ID, Region: f.Region})
				if err != nil {
					findings = append(findings, Finding{Path: f.Path, Message: err.Error()})
					continue
				}
				if !present {
					findings = append(findings, Finding{
						Path:    f.Path,
						Message: fmt.Sprintf("the %s block from %s has been removed", f.Region, entry.ID),
					})
					continue
				}
				if lock.Hash(body) != f.Hash {
					findings = append(findings, Finding{
						Path:    f.Path,
						Message: fmt.Sprintf("the %s block from %s was edited by hand", f.Region, entry.ID),
					})
				}
			}
		}
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].Path < findings[j].Path })
	return findings, nil
}

// ------------------------------------------------------------------ conflicts

// checkConflicts finds files where `--merge-markers` wrote both versions and
// nobody has chosen between them yet.
//
// Writing markers is a deliberate escape hatch, but a half-resolved file is worse
// than either version on its own: agents read it as instructions and humans read
// past it. Leaving it undetected would defeat the point of offering the option.
func checkConflicts(p *engine.Project, _ map[string]any) ([]Finding, error) {
	var findings []Finding
	seen := map[string]bool{}
	for _, entry := range p.Lock.Layers {
		for _, f := range entry.Files {
			if f.Mode == engine.ModeDir || seen[f.Path] {
				continue
			}
			seen[f.Path] = true
			data, err := os.ReadFile(p.Repo.Path(f.Path))
			if err != nil {
				continue
			}
			for i, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, merge.ConflictMarker) {
					findings = append(findings, Finding{
						Path:    f.Path,
						Line:    i + 1,
						Message: "unresolved merge conflict left by --merge-markers",
					})
					break
				}
			}
		}
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].Path < findings[j].Path })
	return findings, nil
}

// --------------------------------------------------------------------- budget

// checkBudget enforces the ceiling on always-on agent instructions.
//
// This exists because unbounded instruction files measurably degrade agent
// performance: published evaluations found generated AGENTS.md files reduced task
// success and raised cost, largely by restating what the repository already
// showed. Detail belongs in skills that load on demand, not in every context
// window.
func checkBudget(p *engine.Project, _ map[string]any) ([]Finding, error) {
	ceiling := p.Config.Budget.Instructions
	if ceiling <= 0 {
		return nil, nil
	}

	in, err := p.TargetInput()
	if err != nil {
		return nil, err
	}

	total := 0
	type contribution struct {
		layer  string
		tokens int
	}
	var parts []contribution
	for _, l := range in.Layers {
		layerTotal := 0
		for _, d := range l.Docs {
			layerTotal += estimateTokens(d.Body)
		}
		if layerTotal > 0 {
			parts = append(parts, contribution{l.ID, layerTotal})
			total += layerTotal
		}
	}
	if total <= ceiling {
		return nil, nil
	}

	sort.Slice(parts, func(i, j int) bool { return parts[i].tokens > parts[j].tokens })
	var breakdown []string
	for _, c := range parts {
		breakdown = append(breakdown, fmt.Sprintf("%s ~%d", c.layer, c.tokens))
	}
	return []Finding{{
		Path:    "AGENTS.md",
		Message: fmt.Sprintf("always-on instructions are ~%d tokens, over the %d ceiling (%s)", total, ceiling, strings.Join(breakdown, ", ")),
	}}, nil
}

// estimateTokens is the usual four-characters-per-token approximation. It only
// needs to be good enough to catch a layer that has quietly grown a handbook.
func estimateTokens(s string) int {
	return (len(s) + 3) / 4
}

// -------------------------------------------------------------------- helpers

// repoRel renders an absolute path the way the lockfile and the baseline store
// it: relative to the repository root, with forward slashes.
func repoRel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func markdownFiles(dir string) ([]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	var out []string
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// parseFrontmatterStrict is parseFrontmatter with the failure reason preserved.
// The distinction matters: "no frontmatter" and "frontmatter that does not parse"
// have completely different fixes, and a check that conflates them sends the
// reader looking for the wrong problem. An unquoted colon in a title is by far
// the most common cause.
func parseFrontmatterStrict(content string) (map[string]any, error) {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return nil, errors.New("no YAML frontmatter")
	}
	rest := content[strings.Index(content, "\n")+1:]
	end := -1
	offset := 0
	for _, line := range strings.Split(rest, "\n") {
		if strings.TrimSpace(line) == "---" {
			end = offset
			break
		}
		offset += len(line) + 1
	}
	if end < 0 {
		return nil, errors.New("the frontmatter block is never closed with `---`")
	}
	var meta map[string]any
	if err := yaml.Unmarshal([]byte(rest[:end]), &meta); err != nil {
		return nil, fmt.Errorf("the frontmatter is not valid YAML (%s) — a value containing a colon must be quoted", firstLine(err.Error()))
	}
	if meta == nil {
		meta = map[string]any{}
	}
	return meta, nil
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i]
	}
	return s
}

// parseFrontmatter extracts a leading YAML block delimited by `---` lines.
func parseFrontmatter(content string) (map[string]any, bool) {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return nil, false
	}
	rest := content[strings.Index(content, "\n")+1:]
	end := -1
	offset := 0
	for _, line := range strings.Split(rest, "\n") {
		if strings.TrimSpace(line) == "---" {
			end = offset
			break
		}
		offset += len(line) + 1
	}
	if end < 0 {
		return nil, false
	}
	var meta map[string]any
	if err := yaml.Unmarshal([]byte(rest[:end]), &meta); err != nil {
		return nil, false
	}
	if meta == nil {
		meta = map[string]any{}
	}
	return meta, true
}

func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006/01/02", "02 Jan 2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case time.Time:
		return t.Format("2006-01-02")
	default:
		return fmt.Sprintf("%v", t)
	}
}

func isBlank(v any) bool { return strings.TrimSpace(asString(v)) == "" }

func stringSlice(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, asString(item))
		}
		return out
	case []string:
		return t
	case string:
		return []string{t}
	}
	return nil
}

func stringSet(v any) map[string]bool {
	out := map[string]bool{}
	for _, s := range stringSlice(v) {
		out[s] = true
	}
	return out
}
