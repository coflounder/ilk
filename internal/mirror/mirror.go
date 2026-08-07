// Package mirror keeps record documents in agreement with an external tracker.
//
// The discipline is the one infrastructure tools have used for a decade, and the
// one ilk already uses on the filesystem: you see the whole plan before anything
// executes, and applying is a separate deliberate step. A write to somebody
// else's system is either correct or it does not happen.
//
// ilk supplies what is the same for every tracker — identity, diffing, refusing
// on ambiguity, writing the remote id back into a key it owns. The layer supplies
// three commands that know the provider and normalise it to the shape below, so
// ilk never learns what a GitHub Project is, exactly as it never learns what
// pytest is.
package mirror

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/manifest"
)

// Item is a remote entry, normalised by the layer's list command.
type Item struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status,omitempty"`
	URL    string `json:"url,omitempty"`
}

// Doc is a record document participating in a mirror.
type Doc struct {
	Path   string `json:"path"`
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status,omitempty"`
	// RemoteID is what the owned frontmatter key holds, empty when unlinked.
	RemoteID string `json:"remote_id,omitempty"`
}

// Op is what a planned action would do.
type Op string

const (
	OpCreate Op = "create"
	OpUpdate Op = "update"
	// OpLink records an existing remote item's id on a document, without
	// changing either side. It is how a repository adopts a tracker that already
	// has content.
	OpLink Op = "link"
	// OpOrphan is a document whose remote counterpart has disappeared. ilk
	// reports it and does nothing: recreating it would duplicate work somebody
	// deliberately closed, and clearing the id would hide that.
	OpOrphan Op = "orphan"
	// OpUntracked is a remote item with no document. ilk never deletes remotely.
	OpUntracked Op = "untracked"
	// OpAmbiguous is a document that could match more than one remote item.
	OpAmbiguous Op = "ambiguous"
)

// Action is one planned operation.
type Action struct {
	Op     Op     `json:"op"`
	Path   string `json:"path,omitempty"`
	DocID  string `json:"doc_id,omitempty"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	// Status is what the record says, carried on every action derived from a
	// document. A create has no diff to recover it from, and an item that lands
	// in the tracker with no status is one the next run has to go back and fix.
	Status string `json:"status,omitempty"`

	RemoteID string `json:"remote_id,omitempty"`
	URL      string `json:"url,omitempty"`
	// Candidates names every remote item an ambiguous document could mean.
	Candidates []Item `json:"candidates,omitempty"`
	// Changes lists the fields that differ, for an update.
	Changes []Change `json:"changes,omitempty"`
}

// Change is one field disagreeing between the record and the tracker.
type Change struct {
	Field  string `json:"field"`
	Record string `json:"record"`
	Remote string `json:"remote"`
}

// Plan is a whole reconciliation.
type Plan struct {
	Mirror  string   `json:"mirror"`
	Actions []Action `json:"actions"`
}

// Writes returns the actions that would change something.
func (p *Plan) Writes() []Action {
	var out []Action
	for _, a := range p.Actions {
		switch a.Op {
		case OpCreate, OpUpdate, OpLink:
			out = append(out, a)
		}
	}
	return out
}

// Blocked returns the actions ilk refuses to take, and the reasons.
func (p *Plan) Blocked() []Action {
	var out []Action
	for _, a := range p.Actions {
		if a.Op == OpAmbiguous {
			out = append(out, a)
		}
	}
	return out
}

// Reports returns what ilk noticed but will not act on.
func (p *Plan) Reports() []Action {
	var out []Action
	for _, a := range p.Actions {
		switch a.Op {
		case OpOrphan, OpUntracked:
			out = append(out, a)
		}
	}
	return out
}

// Empty reports whether applying would do nothing.
func (p *Plan) Empty() bool { return len(p.Writes()) == 0 }

// Options tunes planning.
type Options struct {
	// Link matches unlinked documents to existing remote items by title. It is
	// the adoption path, kept separate from ordinary syncing because guessing
	// identity is a different kind of act from maintaining it.
	Link bool
	// Timeout bounds each provider command.
	Timeout time.Duration
}

// Find returns a mirror declared by an adopted layer.
func Find(p *engine.Project, id string) (*engine.ResolvedLayer, manifest.Mirror, error) {
	var found []manifest.Mirror
	var owners []*engine.ResolvedLayer
	for _, l := range p.Layers {
		for _, m := range l.Loaded.Manifest.Mirrors {
			if id == "" || m.ID == id {
				found = append(found, m)
				owners = append(owners, l)
			}
		}
	}
	switch {
	case len(found) == 0 && id == "":
		return nil, manifest.Mirror{}, fmt.Errorf("no layer here declares a mirror — `ilk search tracker` shows layers that do")
	case len(found) == 0:
		return nil, manifest.Mirror{}, fmt.Errorf("no mirror called %q — %s", id, availableMirrors(p))
	case len(found) > 1:
		return nil, manifest.Mirror{}, fmt.Errorf("more than one mirror is declared; name one — %s", availableMirrors(p))
	}
	return owners[0], found[0], nil
}

func availableMirrors(p *engine.Project) string {
	var names []string
	for _, l := range p.Layers {
		for _, m := range l.Loaded.Manifest.Mirrors {
			names = append(names, m.ID)
		}
	}
	if len(names) == 0 {
		return "no mirrors are declared here"
	}
	sort.Strings(names)
	return "available: " + strings.Join(names, ", ")
}

// Build computes what reconciliation would do.
func Build(p *engine.Project, l *engine.ResolvedLayer, m manifest.Mirror, opts Options) (*Plan, error) {
	docs, err := readDocs(p, l, m)
	if err != nil {
		return nil, err
	}
	items, err := listRemote(p, l, m, opts)
	if err != nil {
		return nil, err
	}

	byID := map[string]Item{}
	for _, item := range items {
		byID[item.ID] = item
	}
	// Titles can repeat remotely; that is precisely the ambiguity worth refusing.
	byTitle := map[string][]Item{}
	for _, item := range items {
		key := normaliseTitle(item.Title)
		byTitle[key] = append(byTitle[key], item)
	}

	plan := &Plan{Mirror: m.ID}
	claimed := map[string]bool{}

	for _, doc := range docs {
		switch {
		case doc.RemoteID != "":
			item, ok := byID[doc.RemoteID]
			if !ok {
				plan.Actions = append(plan.Actions, Action{
					Op: OpOrphan, Path: doc.Path, DocID: doc.ID, Title: doc.Title,
					Status: doc.Status, RemoteID: doc.RemoteID,
					Detail: "the tracker has no item with this id any more",
				})
				continue
			}
			claimed[item.ID] = true
			if changes := diff(doc, item); len(changes) > 0 {
				plan.Actions = append(plan.Actions, Action{
					Op: OpUpdate, Path: doc.Path, DocID: doc.ID, Title: doc.Title,
					Status: doc.Status, RemoteID: item.ID, URL: item.URL, Changes: changes,
				})
			}

		case opts.Link:
			candidates := byTitle[normaliseTitle(doc.Title)]
			switch len(candidates) {
			case 0:
				plan.Actions = append(plan.Actions, Action{
					Op: OpCreate, Path: doc.Path, DocID: doc.ID, Title: doc.Title,
					Status: doc.Status,
					Detail: "no item in the tracker has this title",
				})
			case 1:
				claimed[candidates[0].ID] = true
				plan.Actions = append(plan.Actions, Action{
					Op: OpLink, Path: doc.Path, DocID: doc.ID, Title: doc.Title,
					Status: doc.Status, RemoteID: candidates[0].ID, URL: candidates[0].URL,
				})
			default:
				// Refuse, and name them. A wrong link is silent and permanent:
				// every later sync writes to the wrong item.
				plan.Actions = append(plan.Actions, Action{
					Op: OpAmbiguous, Path: doc.Path, DocID: doc.ID, Title: doc.Title,
					Status: doc.Status, Candidates: candidates,
					Detail: fmt.Sprintf("%d items in the tracker share this title", len(candidates)),
				})
			}

		default:
			plan.Actions = append(plan.Actions, Action{
				Op: OpCreate, Path: doc.Path, DocID: doc.ID, Title: doc.Title,
				Status: doc.Status,
			})
		}
	}

	for _, item := range items {
		if !claimed[item.ID] {
			plan.Actions = append(plan.Actions, Action{
				Op: OpUntracked, Title: item.Title, RemoteID: item.ID, URL: item.URL,
				Detail: "in the tracker, with nothing in the record pointing at it",
			})
		}
	}

	sort.SliceStable(plan.Actions, func(i, j int) bool {
		return opRank(plan.Actions[i].Op) < opRank(plan.Actions[j].Op)
	})
	return plan, nil
}

func opRank(op Op) int {
	switch op {
	case OpAmbiguous:
		return 0
	case OpCreate:
		return 1
	case OpUpdate:
		return 2
	case OpLink:
		return 3
	case OpOrphan:
		return 4
	}
	return 5
}

// diff compares the fields the record owns against the tracker.
//
// The record is the source of truth: this is "make the tracker match the
// markdown", never the other way round. A tracker that has drifted is reported as
// a change to push, not as an update to pull.
func diff(doc Doc, item Item) []Change {
	var out []Change
	if strings.TrimSpace(doc.Title) != strings.TrimSpace(item.Title) {
		out = append(out, Change{Field: "title", Record: doc.Title, Remote: item.Title})
	}
	if doc.Status != "" && !strings.EqualFold(doc.Status, item.Status) {
		out = append(out, Change{Field: "status", Record: doc.Status, Remote: item.Status})
	}
	return out
}

// normaliseTitle makes title matching forgiving about case and spacing, and no
// more forgiving than that. Fuzzier matching would produce confident wrong links.
func normaliseTitle(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

// readDocs loads the record documents taking part in this mirror.
func readDocs(p *engine.Project, l *engine.ResolvedLayer, m manifest.Mirror) ([]Doc, error) {
	dir, err := renderIn(l, m.Dir)
	if err != nil {
		return nil, err
	}
	// Match is rendered like every other manifest string. Compiling it raw would
	// turn a layer variable into a regex that matches nothing, and a mirror that
	// silently sees no documents looks exactly like a mirror with nothing to do.
	match, err := renderIn(l, m.Match)
	if err != nil {
		return nil, err
	}
	matcher := func(string) bool { return true }
	if match != "" {
		re, err := regexp.Compile(match)
		if err != nil {
			return nil, fmt.Errorf("mirror %s: match: %w", m.ID, err)
		}
		matcher = re.MatchString
	}

	abs := p.Repo.Path(dir)
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, fmt.Errorf("mirror %s: cannot read %s: %w", m.ID, dir, err)
	}

	var docs []Doc
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.EqualFold(filepath.Ext(name), ".md") || name == "README.md" || !matcher(name) {
			continue
		}
		meta, err := frontmatter(filepath.Join(abs, name))
		if err != nil {
			// The match pattern already said this document belongs to the mirror,
			// so dropping it here would take it off the tracker without saying so —
			// and the tracker would look complete while missing it.
			return nil, fmt.Errorf("mirror %s: %s matches this mirror but its frontmatter cannot be read: %w\n  fix: give it a `---` block with at least a title, or narrow the mirror's `match` so it is not included", m.ID, filepath.ToSlash(filepath.Join(dir, name)), err)
		}
		doc := Doc{
			Path:     filepath.ToSlash(filepath.Join(dir, name)),
			ID:       scalar(meta["id"]),
			Title:    scalar(meta["title"]),
			Status:   scalar(meta["status"]),
			RemoteID: remoteID(meta[m.Key]),
		}
		if doc.Title == "" {
			doc.Title = strings.TrimSuffix(name, filepath.Ext(name))
		}
		docs = append(docs, doc)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	return docs, nil
}

// remoteID reads the id out of the key ilk owns. Both shapes are accepted: a
// bare string, and a mapping with an `id` — the second is what ilk writes, since
// it leaves room for the url alongside.
func remoteID(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case map[string]any:
		return strings.TrimSpace(scalar(t["id"]))
	}
	return ""
}

// listRemote runs the layer's list command and parses the normalised array.
func listRemote(p *engine.Project, l *engine.ResolvedLayer, m manifest.Mirror, opts Options) ([]Item, error) {
	command, err := renderIn(l, m.List)
	if err != nil {
		return nil, err
	}
	out, err := run(p, l, command, "", opts.Timeout)
	if err != nil {
		return nil, fmt.Errorf("mirror %s: listing the tracker failed: %w", m.ID, err)
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil, nil
	}
	var items []Item
	if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil, fmt.Errorf("mirror %s: the list command did not print a JSON array of {id, title, status, url}: %w", m.ID, err)
	}
	for i, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			return nil, fmt.Errorf("mirror %s: item %d from the tracker has no id, so nothing can be matched to it", m.ID, i)
		}
	}
	return items, nil
}

// run executes a provider command.
//
// The payload is offered twice: as JSON on stdin for a provider that wants
// structure, and as ILK_MIRROR_* environment variables for one written in shell.
// The second is not a convenience — it means a provider script never has to parse
// JSON, which is where shell integrations usually acquire their bugs and their
// dependency on jq.
func run(p *engine.Project, l *engine.ResolvedLayer, command, stdin string, timeout time.Duration, env ...string) (string, error) {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = p.Repo.Root
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "ILK_LAYER="+l.ID(), "ILK_REPO_ROOT="+p.Repo.Root)
	for k, v := range l.Vars {
		cmd.Env = append(cmd.Env, "ILK_VAR_"+strings.ToUpper(k)+"="+v)
	}
	cmd.Env = append(cmd.Env, env...)

	done := make(chan struct{})
	var out []byte
	var err error
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
		return "", fmt.Errorf("timed out after %s", timeout)
	}
	return string(out), err
}
