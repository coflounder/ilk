// Package checks runs every validator an adopted layer registers, plus the few
// ilk always runs itself.
//
// The design rule, taken from the principle that agents must be able to repair
// their own work: every failure carries a fix precise enough to act on without a
// human. A check that can only say "invalid" is a bug in the check.
package checks

import (
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
	ID       string        `json:"id"`
	Title    string        `json:"title,omitempty"`
	Layer    string        `json:"layer"`
	Status   Status        `json:"status"`
	Findings []Finding     `json:"findings,omitempty"`
	Fix      string        `json:"fix,omitempty"`
	Reason   string        `json:"reason,omitempty"`
	Output   string        `json:"output,omitempty"`
	Duration time.Duration `json:"duration_ms"`
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
	for _, id := range append(append([]string{}, opts.Only...), opts.Skip...) {
		if !known[id] {
			available := make([]string, 0, len(known))
			for k := range known {
				available = append(available, k)
			}
			sort.Strings(available)
			return nil, fmt.Errorf("no check called %q — available: %s", id, strings.Join(available, ", "))
		}
	}
	only := map[string]bool{}
	for _, id := range opts.Only {
		only[id] = true
	}
	skip := map[string]bool{}
	for _, id := range opts.Skip {
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
			res.Reason = fmt.Sprintf("needs the %s capability — set it under `capabilities:` in .ilk/config.yaml to enable this check", r.check.Requires)
			res.Duration = time.Since(start)
			return res
		}
	}

	// A credential lives in the environment, so without this the command runs,
	// fails to authenticate, and reports a broken repository. Skipping says what
	// is actually wrong. The values are never read — only their presence.
	if missing := missingEnv(r.check.RequiresEnv); len(missing) > 0 {
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
		out, code, err := runCommand(p.Repo.Root, cmd, opts.Timeout, checkEnv(r))
		res.Output = strings.TrimRight(out, "\n")
		switch {
		case err != nil:
			res.Status = StatusError
			res.Reason = err.Error()
		case code == 0:
			res.Status = StatusPass
		default:
			res.Status = StatusFail
			res.Findings = []Finding{{Message: fmt.Sprintf("`%s` exited %d", cmd, code)}}
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

func runCommand(dir, command string, timeout time.Duration, env []string) (string, int, error) {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.CombinedOutput()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return string(out), -1, fmt.Errorf("timed out after %s", timeout)
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		return string(out), exitErr.ExitCode(), nil
	}
	if err != nil {
		return string(out), -1, err
	}
	return string(out), 0, nil
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
