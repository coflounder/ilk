package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/coflounder/ilk/internal/checks"
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/layer"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Check assertions: the half of `ilk layer test` that was missing.
//
// The round trip proves a layer can be added and taken back. It says nothing
// about whether the layer's checks reject what they claim to, and a check whose
// pattern matches nothing looks exactly like a check that passes — the same
// failure `record.coverage` exists to catch in documents, which ilk was not
// applying to itself.
//
// A layer states its cases in test/checks.yaml:
//
//	- check: blueprint.epic
//	  fixture: spec-with-missing-epic
//	  expect: fail
//	- check: blueprint.epic
//	  fixture: spec-with-real-epic
//	  expect: pass
//
// Each fixture is a directory under test/fixtures/ that is copied over the
// sandbox before the named check runs alone. Both directions are required:
// a check that fails on everything is as broken as one that fails on nothing,
// and only the second is otherwise noticeable.

// assertionFile is where a layer keeps its check cases.
const assertionFile = "test/checks.yaml"

// assertion is one case: plant this, run that check, expect this verdict.
type assertion struct {
	Check   string `yaml:"check"`
	Fixture string `yaml:"fixture"`
	Expect  string `yaml:"expect"`
	// Why records what the case is defending, for the failure message. A case
	// nobody can interpret is as useless as a check nobody can act on.
	Why string `yaml:"why,omitempty"`
}

// assertionResult is one case, run.
type assertionResult struct {
	Check   string `json:"check"`
	Fixture string `json:"fixture"`
	Expect  string `json:"expect"`
	Got     string `json:"got"`
	Why     string `json:"why,omitempty"`
	Passed  bool   `json:"passed"`
	Detail  string `json:"detail,omitempty"`
}

// loadAssertions reads a layer's check cases. A layer with none is not an error
// here — CI decides whether that is acceptable, not the runner.
func loadAssertions(l *layer.Loaded) ([]assertion, error) {
	data, err := fs.ReadFile(l.FS, assertionFile)
	if err != nil {
		return nil, nil
	}
	var out []assertion
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", assertionFile, err)
	}

	declared := map[string]bool{}
	for _, c := range l.Manifest.Checks {
		declared[c.ID] = true
	}
	for i, a := range out {
		where := fmt.Sprintf("%s[%d]", assertionFile, i)
		if a.Check == "" {
			return nil, fmt.Errorf("%s: check is required", where)
		}
		if !declared[a.Check] {
			return nil, fmt.Errorf("%s: this layer declares no check %q — the case would test nothing", where, a.Check)
		}
		if a.Fixture == "" {
			return nil, fmt.Errorf("%s (%s): fixture is required — name a directory under test/fixtures/", where, a.Check)
		}
		if a.Expect != "pass" && a.Expect != "fail" {
			return nil, fmt.Errorf("%s (%s): expect must be `pass` or `fail`, got %q", where, a.Check, a.Expect)
		}
	}
	return out, nil
}

// runAssertions plants each fixture into the sandbox in turn and runs the named
// check by itself.
//
// Fixtures are planted and then removed again, so one case cannot leak into the
// next — a fixture that made a later case pass would be the most confusing
// possible outcome here.
func runAssertions(sandbox string, l *layer.Loaded, cases []assertion) ([]assertionResult, error) {
	var out []assertionResult
	for _, a := range cases {
		planted, err := plantFixture(sandbox, l, a.Fixture)
		if err != nil {
			return nil, err
		}

		p, err := engine.Load(sandbox, "test")
		if err != nil {
			return nil, err
		}
		report, err := checks.Run(p, checks.Options{Only: []string{a.Check}})
		if err != nil {
			return nil, err
		}

		got, detail := verdictOf(report, a.Check)
		res := assertionResult{
			Check: a.Check, Fixture: a.Fixture, Expect: a.Expect,
			Got: got, Why: a.Why, Passed: got == a.Expect, Detail: detail,
		}
		out = append(out, res)

		for _, rel := range planted {
			_ = os.RemoveAll(filepath.Join(sandbox, rel))
		}
	}
	return out, nil
}

// verdictOf reduces a report to pass or fail for one check. A check that could
// not run is neither, and says so rather than being counted as either.
func verdictOf(report *checks.Report, id string) (string, string) {
	for _, r := range report.Results {
		if r.ID != id {
			continue
		}
		switch r.Status {
		case checks.StatusPass:
			return "pass", ""
		case checks.StatusFail:
			return "fail", firstFindingText(r)
		case checks.StatusSkip:
			return "skipped", r.Reason
		default:
			return "errored", r.Reason
		}
	}
	return "missing", "the check did not run"
}

func firstFindingText(r checks.Result) string {
	if len(r.Findings) == 0 {
		return r.Reason
	}
	f := r.Findings[0]
	if f.Path == "" {
		return f.Message
	}
	return f.Path + ": " + f.Message
}

// plantFixture copies a fixture directory into the sandbox, returning the
// top-level entries it created so they can be taken out again.
func plantFixture(sandbox string, l *layer.Loaded, name string) ([]string, error) {
	root := "test/fixtures/" + name
	entries, err := fs.ReadDir(l.FS, root)
	if err != nil {
		return nil, fmt.Errorf("fixture %q not found at %s/ in the layer", name, root)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("fixture %q is empty, so the case would prove nothing", name)
	}

	var top []string
	for _, e := range entries {
		top = append(top, e.Name())
	}

	err = fs.WalkDir(l.FS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		dest := filepath.Join(sandbox, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		data, readErr := fs.ReadFile(l.FS, p)
		if readErr != nil {
			return readErr
		}
		if mkErr := os.MkdirAll(filepath.Dir(dest), 0o755); mkErr != nil {
			return mkErr
		}
		return os.WriteFile(dest, data, 0o644)
	})
	return top, err
}

// printAssertions renders the cases under the round-trip results.
func printAssertions(results []assertionResult) {
	for _, r := range results {
		label := fmt.Sprintf("%s %s on %s", r.Check, expectVerb(r.Expect), r.Fixture)
		printf("  %s %s\n", pass(r.Passed), label)
		if r.Passed {
			continue
		}
		printf("      %s got %s\n", sty.red("but"), r.Got)
		if r.Detail != "" {
			printf("      %s\n", sty.dim(r.Detail))
		}
		if r.Why != "" {
			printf("      %s %s\n", sty.dim("defends:"), sty.dim(r.Why))
		}
	}
}

func expectVerb(expect string) string {
	if expect == "fail" {
		return "rejects"
	}
	return "accepts"
}

// assertionsFailed reports whether any case did not get the verdict it wanted.
func assertionsFailed(results []assertionResult) bool {
	for _, r := range results {
		if !r.Passed {
			return true
		}
	}
	return false
}

// uncheckedChecks lists checks a layer registers but never asserts. Reported
// rather than failed by default: a layer author should see the gap without
// `ilk layer test` refusing to run.
func uncheckedChecks(l *layer.Loaded, cases []assertion) []string {
	covered := map[string]bool{}
	for _, a := range cases {
		covered[a.Check] = true
	}
	var out []string
	for _, c := range l.Manifest.Checks {
		if !covered[c.ID] {
			out = append(out, c.ID)
		}
	}
	return out
}

// addStrictFlag is defined here so the flag's meaning lives beside the thing it
// governs.
func addStrictFlag(cmd *cobra.Command, strict *bool) {
	cmd.Flags().BoolVar(strict, "strict", false,
		"fail if any check has no case in "+assertionFile+" (what CI holds this repository's layers to)")
}

// strictComplaint renders the refusal when --strict finds an unasserted check.
func strictComplaint(missing []string) string {
	return fmt.Sprintf("%d check(s) have no case in %s: %s\n"+
		"  A check nobody tests is indistinguishable from a check that matches nothing.\n"+
		"  fix: add a fixture under test/fixtures/ and a case asserting the check rejects it",
		len(missing), assertionFile, strings.Join(missing, ", "))
}

// providersFor maps each required capability to a built-in layer that provides
// it, so `ilk layer test` can compose rather than stub.
//
// Only built-in layers are considered: they resolve with no network and no
// configuration, which is what makes this safe to do unconditionally. A
// requirement nothing built in satisfies falls back to a stub.
func providersFor(requires []string) (map[string]string, error) {
	if len(requires) == 0 {
		return nil, nil
	}
	builtins, err := layer.Builtins()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, req := range requires {
		for _, b := range builtins {
			if _, ok := b.Manifest.Provides[req]; ok {
				out[req] = b.Manifest.ID
				break
			}
		}
	}
	return out, nil
}

// sortedValues lists the distinct values of a map in a stable order, so the
// sandbox is built the same way every run.
func sortedValues(m map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range m {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
