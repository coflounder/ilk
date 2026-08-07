package contrib

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/render"
)

// DefaultSubmit is the command used when a layer declares no other.
//
// It is shipped by the toolkit layer, which every `ilk init` adopts, so a layer
// that names a GitHub repository is contributable with no further setup. A layer
// hosted elsewhere overrides `contribution.submit` and nothing in ilk changes.
const DefaultSubmit = ".ilk/bin/ilk-propose.sh"

// Draft writes the proposal document, without sending anything.
//
// The document is a file rather than a pull-request body held in memory because
// the useful next step is an agent editing it — filling in the judgement, cutting
// the evidence that turned out not to matter. That is much easier against a file
// than against a prompt, and it means a half-written proposal survives closing the
// terminal.
func Draft(p *engine.Project, prop *Proposal) (string, error) {
	rel := prop.Path()
	abs := p.Repo.Path(rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); err == nil {
		// Overwriting would throw away judgement somebody already wrote, which is
		// the only part of the document that took effort.
		return rel, fmt.Errorf("%s already exists — edit it, or delete it to gather the evidence again", rel)
	}
	if err := os.WriteFile(abs, []byte(prop.Render()), 0o644); err != nil {
		return "", err
	}
	return rel, nil
}

// Ready reports what still stands between a draft and a pull request.
func Ready(p *engine.Project, prop *Proposal) (document string, blockers []string, err error) {
	rel := prop.Path()
	data, err := os.ReadFile(p.Repo.Path(rel))
	if err != nil {
		return "", nil, fmt.Errorf("no draft at %s — run `ilk contribute %s` first", rel, prop.Layer)
	}
	document = string(data)

	for _, line := range unwritten(document) {
		blockers = append(blockers, fmt.Sprintf("%s still says: %s", rel, line))
	}
	for _, c := range prop.Blocking() {
		blockers = append(blockers, fmt.Sprintf("%s line %d: %s", c.Path, c.Line, c.Reason))
	}
	if prop.Contribution == nil {
		blockers = append(blockers, fmt.Sprintf("%s declares no `contribution:` block, so there is nowhere to send this. Its maintainer has not opted in", prop.Layer))
	} else if prop.Contribution.Repo == "" {
		blockers = append(blockers, fmt.Sprintf("%s does not say which repository it comes from", prop.Layer))
	}
	return document, blockers, nil
}

// Submit hands the finished proposal to the command that opens it upstream.
//
// ilk does not talk to GitHub. The same split as a mirror: core owns the part that
// is true of every forge — what a proposal is, when it is ready, what must never
// be in one — and a command owns the part that knows about pull requests. That is
// also what makes this testable without an account.
func Submit(p *engine.Project, l *engine.ResolvedLayer, prop *Proposal, document string, timeout time.Duration) (string, error) {
	c := prop.Contribution
	command := c.Submit
	if command == "" {
		command = DefaultSubmit
	}
	command, err := render.String("contribution:submit", command, l.Ctx)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(command, ".ilk/") {
		if _, err := os.Stat(p.Repo.Path(command)); err != nil {
			return "", fmt.Errorf("%s is missing. It ships with the ilk/toolkit layer — `ilk add toolkit` restores it, or set `contribution.submit` in the layer to a command of your own", command)
		}
	}

	base := c.Base
	if base == "" {
		base = "main"
	}
	branch := fmt.Sprintf("ilk-proposal/%s", strings.ReplaceAll(prop.Layer, "/", "-"))

	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = p.Repo.Root
	cmd.Stdin = strings.NewReader(opened(document))
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"ILK_LAYER="+prop.Layer,
		"ILK_REPO_ROOT="+p.Repo.Root,
		"ILK_PROPOSAL_PATH="+prop.Path(),
		"ILK_PROPOSAL_TITLE="+prop.Title(),
		"ILK_PROPOSAL_BRANCH="+branch,
		"ILK_PROPOSAL_FROM="+prop.Repo,
		"ILK_PROPOSAL_SLUG="+strings.ReplaceAll(prop.Layer, "/", "-"),
		"ILK_CONTRIB_REPO="+c.Repo,
		"ILK_CONTRIB_PATH="+c.Path,
		"ILK_CONTRIB_BASE="+base,
	)
	for k, v := range l.Vars {
		cmd.Env = append(cmd.Env, "ILK_VAR_"+strings.ToUpper(k)+"="+v)
	}

	done := make(chan struct{})
	var out []byte
	go func() {
		out, err = cmd.Output()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return "", fmt.Errorf("the submit command timed out after %s", timeout)
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// opened marks the document as no longer a draft.
//
// The word matters at the other end: upstream's checks distinguish a proposal
// waiting for a decision from one that has had one, and a queue full of things
// still called drafts tells a maintainer nothing about what needs them.
func opened(document string) string {
	return strings.Replace(document, "\nstatus: draft\n", "\nstatus: open\n", 1)
}

// Guidelines reads what this layer asks of a proposal, from the layer's own tree.
//
// Standards a contributor only learns in review are standards that waste
// everybody's time. Shipping them inside the layer means the adopter has them
// locally, before writing a word.
func Guidelines(l *engine.ResolvedLayer) (string, bool) {
	c := l.Loaded.Manifest.Contribution
	if c == nil || c.Guidelines == "" {
		return "", false
	}
	text, err := readLayerFile(l, c.Guidelines)
	if err != nil {
		return "", false
	}
	return text, true
}
