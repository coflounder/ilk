// Package checks runs every validator an adopted layer registers, plus the few
// ilk always runs itself.
//
// The design rule, taken from the principle that agents must be able to repair
// their own work: every failure carries a fix precise enough to act on without a
// human. A check that can only say "invalid" is a bug in the check.
package checks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/coflounder/ilk/internal/render"
)

// Status is a check's outcome.
type Status string

const (
	StatusPass Status = "pass"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
	// StatusError means the check itself could not run, which is different from
	// the repository being wrong and must not be reported as a failure.
	StatusError Status = "error"
)

// Finding is one concrete problem, located precisely enough to act on.
type Finding struct {
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

// Result is one check's outcome.
type Result struct {
	ID       string    `json:"id"`
	Title    string    `json:"title,omitempty"`
	Layer    string    `json:"layer"`
	Status   Status    `json:"status"`
	Findings []Finding `json:"findings,omitempty"`
	Fix      string    `json:"fix,omitempty"`
	Reason   string    `json:"reason,omitempty"`
	Output   string    `json:"output,omitempty"`
	// Warnings are about the check rather than the repository: a line of
	// `findings: jsonl` output that could not be read, for instance.
	Warnings []string      `json:"warnings,omitempty"`
	Duration time.Duration `json:"-"`
}

// MarshalJSON reports Duration as real milliseconds. A time.Duration marshals as
// nanoseconds, which a field named duration_ms used to pass off as milliseconds.
func (r Result) MarshalJSON() ([]byte, error) {
	type plain Result
	return json.Marshal(struct {
		plain
		DurationMS float64 `json:"duration_ms"`
	}{plain(r), float64(r.Duration.Microseconds()) / 1000})
}

// Report is a whole run.
type Report struct {
	Results []Result `json:"results"`
	Passed  int      `json:"passed"`
	Failed  int      `json:"failed"`
	Skipped int      `json:"skipped"`
	Errored int      `json:"errored"`
}

// OK reports whether the run should be treated as success.
func (r *Report) OK() bool { return r.Failed == 0 && r.Errored == 0 }

// registered is a check bound to the layer that contributed it.
type registered struct {
	check manifest.Check
	layer string
	ctx   render.Context
}

// Options tunes a run.
type Options struct {
	// Only restricts the run to these check ids. Unknown ids are an error, so a
	// typo in a hook does not silently run nothing.
	Only []string
	// Skip excludes ids.
	Skip []string
	// Timeout bounds each command-based check.
	Timeout time.Duration
}

// Run executes the selected checks against a project.
func Run(p *engine.Project, opts Options) (*Report, error) {
	all, err := collect(p)
	if err != nil {
		return nil, err
	}

	selected, err := filter(all, opts)
	if err != nil {
		return nil, err
	}

	caps := p.Capabilities()
	report := &Report{}
	for _, r := range selected {
		result := runOne(p, r, caps, opts)
		report.Results = append(report.Results, result)
		switch result.Status {
		case StatusPass:
			report.Passed++
		case StatusFail:
			report.Failed++
		case StatusSkip:
			report.Skipped++
		case StatusError:
			report.Errored++
		}
	}
	return report, nil
}

// IDs lists every check id available in a project.
func IDs(p *engine.Project) ([]string, error) {
	all, err := collect(p)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(all))
	for _, r := range all {
		ids = append(ids, r.check.ID)
	}
	sort.Strings(ids)
	return ids, nil
}

func collect(p *engine.Project) ([]registered, error) {
	var out []registered
	for _, c := range coreChecks() {
		out = append(out, registered{check: c, layer: engine.CoreOwner, ctx: baseContext(p)})
	}
	for _, l := range p.Layers {
		for _, c := range l.Loaded.Manifest.Checks {
			out = append(out, registered{check: c, layer: l.ID(), ctx: l.Ctx})
		}
	}
	seen := map[string]string{}
	for _, r := range out {
		if prev, ok := seen[r.check.ID]; ok {
			return nil, fmt.Errorf("two layers register a check called %q (%s and %s) — check ids must be unique across a repository", r.check.ID, prev, r.layer)
		}
		seen[r.check.ID] = r.layer
	}
	sort.Slice(out, func(i, j int) bool { return out[i].check.ID < out[j].check.ID })
	return out, nil
}

func baseContext(p *engine.Project) render.Context {
	return render.Context{
		Repo: render.RepoInfo{Name: p.Repo.Name(), Root: p.Repo.Root},
		Vars: map[string]string{},
		Caps: p.Capabilities(),
		Ilk:  render.IlkInfo{Version: p.Version},
	}
}

func filter(all []registered, opts Options) ([]registered, error) {
	if len(opts.Only) == 0 && len(opts.Skip) == 0 {
		return all, nil
	}
	known := map[string]bool{}
	for _, r := range all {
		known[r.check.ID] = true
	}
	// A name with no check of its own selects every check of the layer it
	// names, because check ids start with their layer's name.
	expand := func(ids []string) ([]string, error) {
		var out []string
		for _, id := range ids {
			if known[id] {
				out = append(out, id)
				continue
			}
			var layer []string
			for _, r := range all {
				if strings.HasPrefix(r.check.ID, id+".") {
					layer = append(layer, r.check.ID)
				}
			}
			if len(layer) == 0 {
				available := make([]string, 0, len(known))
				for k := range known {
					available = append(available, k)
				}
				sort.Strings(available)
				return nil, fmt.Errorf("no check or layer called %q — available: %s", id, strings.Join(available, ", "))
			}
			out = append(out, layer...)
		}
		return out, nil
	}
	onlyIDs, err := expand(opts.Only)
	if err != nil {
		return nil, err
	}
	skipIDs, err := expand(opts.Skip)
	if err != nil {
		return nil, err
	}
	only := map[string]bool{}
	for _, id := range onlyIDs {
		only[id] = true
	}
	skip := map[string]bool{}
	for _, id := range skipIDs {
		skip[id] = true
	}
	var out []registered
	for _, r := range all {
		if len(only) > 0 && !only[r.check.ID] {
			continue
		}
		if skip[r.check.ID] {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func runOne(p *engine.Project, r registered, caps map[string]string, opts Options) Result {
	start := time.Now()
	res := Result{ID: r.check.ID, Layer: r.layer, Title: r.check.Title}

	fix, err := render.String("fix:"+r.check.ID, r.check.Fix, r.ctx)
	if err != nil {
		res.Status = StatusError
		res.Reason = err.Error()
		return res
	}
	res.Fix = fix

	if r.check.Requires != "" {
		if _, ok := caps[r.check.Requires]; !ok {
			res.Status = StatusSkip
			res.Reason = fmt.Sprintf("needs the %s capability — `ilk config set %s \"<command>\"` enables this check", r.check.Requires, r.check.Requires)
			res.Duration = time.Since(start)
			return res
		}
	}

	// A credential lives in the environment, so without this the command runs,
	// fails to authenticate, and reports a broken repository. Skipping says what
	// is actually wrong. The values are never read — only their presence.
	names, err := requiredEnv(r)
	if err != nil {
		res.Status = StatusError
		res.Reason = err.Error()
		res.Duration = time.Since(start)
		return res
	}
	if missing := missingEnv(names); len(missing) > 0 {
		res.Status = StatusSkip
		res.Reason = fmt.Sprintf("needs %s in the environment — this check cannot tell an absent credential from a failure, so it declines to guess",
			strings.Join(missing, ", "))
		res.Duration = time.Since(start)
		return res
	}

	switch {
	case r.check.Kind != "":
		fn, ok := builtins[r.check.Kind]
		if !ok {
			res.Status = StatusError
			res.Reason = fmt.Sprintf("unknown builtin check kind %q — known kinds: %s", r.check.Kind, strings.Join(builtinNames(), ", "))
			break
		}
		args, err := renderArgs(r.check.Args, r.ctx)
		if err != nil {
			res.Status = StatusError
			res.Reason = err.Error()
			break
		}
		findings, err := fn(p, args)
		if err != nil {
			res.Status = StatusError
			res.Reason = err.Error()
			break
		}
		res.Findings = findings
		res.Status = StatusPass
		if len(findings) > 0 {
			res.Status = StatusFail
		}

	case r.check.Run != "":
		cmd, err := render.String("run:"+r.check.ID, r.check.Run, r.ctx)
		if err != nil {
			res.Status = StatusError
			res.Reason = err.Error()
			break
		}
		jsonl := r.check.Findings == manifest.FindingsJSONL
		stdout, stderr, code, err := runCommand(p.Repo.Root, cmd, opts.Timeout, checkEnv(r), jsonl)
		output := stdout
		if jsonl {
			// Findings come on stdout; anything else the script says is on stderr,
			// and is kept as its output.
			res.Findings, res.Warnings = parseFindings(stdout)
			output = stderr
		}
		res.Output = strings.TrimRight(output, "\n")
		switch {
		case err != nil:
			res.Status = StatusError
			res.Reason = err.Error()
		case code == 0:
			res.Status = StatusPass
			if len(res.Findings) > 0 {
				res.Warnings = append(res.Warnings, fmt.Sprintf("printed %d finding(s) but exited 0, so it passed — a check script exits 1 to fail", len(res.Findings)))
			}
		case code == 1:
			res.Status = StatusFail
			if len(res.Findings) == 0 {
				res.Findings = []Finding{{Message: fmt.Sprintf("`%s` exited 1", shortCommand(cmd))}}
			}
		default:
			res.Status = StatusError
			res.Reason = scriptErrorReason(cmd, code)
		}

	default:
		res.Status = StatusError
		res.Reason = "check declares neither kind nor run"
	}

	res.Duration = time.Since(start)
	return res
}

// checkEnv gives a check script the same environment a layer's commands get.
//
// Without this a check that shells out to a script the layer ships can only
// receive its configuration by interpolating it into the command string, which
// works right up to the first value containing a space.
// requiredEnv renders a check's requires_env entries, dropping any that render
// empty, and refuses a result that is not a variable name.
func requiredEnv(r registered) ([]string, error) {
	var names []string
	for i, entry := range r.check.RequiresEnv {
		name, err := render.String(fmt.Sprintf("requires_env[%d]:%s", i, r.check.ID), entry, r.ctx)
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !manifest.ValidEnvName(name) {
			return nil, fmt.Errorf("requires_env %q renders to %q, which is not an environment variable name — set the layer variable it reads to one, or to nothing", entry, name)
		}
		names = append(names, name)
	}
	return names, nil
}

// missingEnv lists which of the named variables are absent or empty. Only their
// presence is tested; ilk never reads a credential it was asked to notice.
func missingEnv(names []string) []string {
	var missing []string
	for _, name := range names {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

func checkEnv(r registered) []string {
	env := []string{"ILK_LAYER=" + r.layer}
	for k, v := range r.ctx.Vars {
		env = append(env, "ILK_VAR_"+strings.ToUpper(k)+"="+v)
	}
	return env
}

// scriptErrorReason explains an exit code outside the check contract.
func scriptErrorReason(cmd string, code int) string {
	why := "a check script exits 0 to pass and 1 to fail; anything else means the check could not run"
	switch {
	case code == 127:
		why = "127 means the command was not found — is the script there, and is ilk apply up to date?"
	case code == 126:
		why = "126 means the command could not be executed — check its permissions"
	case code > 128:
		why = fmt.Sprintf("it was killed by signal %d", code-128)
	case code < 0:
		why = "it was killed by a signal"
	}
	return fmt.Sprintf("`%s` exited %d: %s", shortCommand(cmd), code, why)
}

// shortCommand is a command as a message can quote it: its first line, cut short.
func shortCommand(cmd string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(cmd), "\n")
	return truncate(line, 72)
}

// parseFindings reads `findings: jsonl` output: one JSON object per line, with
// at least a message. A line that is not one becomes a warning and is otherwise
// skipped, so a stray print in a script costs a warning, never the check.
func parseFindings(stdout string) ([]Finding, []string) {
	var findings []Finding
	var warnings []string
	for i, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var f Finding
		if err := json.Unmarshal([]byte(line), &f); err != nil || strings.TrimSpace(f.Message) == "" {
			warnings = append(warnings, fmt.Sprintf("line %d of its output is not a finding (want {\"path\": ..., \"line\": ..., \"message\": ...}): %s", i+1, truncate(line, 80)))
			continue
		}
		findings = append(findings, f)
	}
	return findings, warnings
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// runCommand runs a check's command. With split, stdout and stderr come back
// separately; otherwise they are interleaved in stdout, as a person would see
// them, and stderr is empty.
func runCommand(dir, command string, timeout time.Duration, env []string, split bool) (stdout, stderr string, code int, err error) {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf
	if split {
		cmd.Stderr = &errBuf
	}

	done := make(chan struct{})
	go func() {
		err = cmd.Run()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return outBuf.String(), errBuf.String(), -1, fmt.Errorf("timed out after %s", timeout)
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		return outBuf.String(), errBuf.String(), exitErr.ExitCode(), nil
	}
	if err != nil {
		return outBuf.String(), errBuf.String(), -1, err
	}
	return outBuf.String(), errBuf.String(), 0, nil
}

// renderArgs walks a check's arguments and renders every string through the
// layer's context, so args can reference the layer's variables.
func renderArgs(args map[string]any, ctx render.Context) (map[string]any, error) {
	out := map[string]any{}
	for k, v := range args {
		rendered, err := renderAny(v, ctx)
		if err != nil {
			return nil, err
		}
		out[k] = rendered
	}
	return out, nil
}

func renderAny(v any, ctx render.Context) (any, error) {
	switch t := v.(type) {
	case string:
		return render.String("arg", t, ctx)
	case []any:
		out := make([]any, 0, len(t))
		for _, item := range t {
			r, err := renderAny(item, ctx)
			if err != nil {
				return nil, err
			}
			out = append(out, r)
		}
		return out, nil
	case map[string]any:
		out := map[string]any{}
		for k, item := range t {
			r, err := renderAny(item, ctx)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	default:
		return v, nil
	}
}
