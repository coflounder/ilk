package mirror

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/manifest"
	"gopkg.in/yaml.v3"
)

// Result is what applying actually did.
type Result struct {
	Created int      `json:"created"`
	Updated int      `json:"updated"`
	Linked  int      `json:"linked"`
	Errors  []string `json:"errors,omitempty"`
}

// Apply executes the writes in a plan.
//
// Ambiguous documents are never applied — they are the plan refusing, and a run
// that hits one still does everything else and leaves a precise list of what it
// would not touch. Anything the provider rejects stops that document and not the
// run: a tracker that half-accepts a batch is a normal Tuesday, and the useful
// outcome is a record of which half.
func Apply(p *engine.Project, l *engine.ResolvedLayer, m manifest.Mirror, plan *Plan, opts Options) (*Result, error) {
	createCmd, err := renderIn(l, m.Create)
	if err != nil {
		return nil, err
	}
	updateCmd, err := renderIn(l, m.Update)
	if err != nil {
		return nil, err
	}

	result := &Result{}
	for _, a := range plan.Writes() {
		switch a.Op {
		case OpCreate:
			payload, _ := json.Marshal(map[string]string{
				"doc_id": a.DocID, "title": a.Title, "path": a.Path,
				"status": a.Status,
			})
			out, err := run(p, l, createCmd, string(payload), opts.Timeout,
				"ILK_MIRROR_TITLE="+a.Title,
				"ILK_MIRROR_STATUS="+a.Status,
				"ILK_MIRROR_DOC_ID="+a.DocID,
				"ILK_MIRROR_PATH="+a.Path)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: creating in the tracker failed: %v", a.Path, err))
				continue
			}
			remoteID, url := parseCreated(out)
			if remoteID == "" {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: the create command printed no id, so nothing can be linked to it", a.Path))
				continue
			}
			if err := writeRemoteID(p, m, a.Path, remoteID, url); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: created %s in the tracker but could not record its id: %v", a.Path, remoteID, err))
				continue
			}
			result.Created++

		case OpUpdate:
			payload, _ := json.Marshal(map[string]any{
				"id": a.RemoteID, "title": a.Title, "doc_id": a.DocID,
				"status": a.Status, "changes": a.Changes,
			})
			if _, err := run(p, l, updateCmd, string(payload), opts.Timeout,
				"ILK_MIRROR_ID="+a.RemoteID,
				"ILK_MIRROR_TITLE="+a.Title,
				"ILK_MIRROR_STATUS="+a.Status,
				"ILK_MIRROR_DOC_ID="+a.DocID,
				"ILK_MIRROR_PATH="+a.Path); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: updating %s failed: %v", a.Path, a.RemoteID, err))
				continue
			}
			result.Updated++

		case OpLink:
			if err := writeRemoteID(p, m, a.Path, a.RemoteID, a.URL); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", a.Path, err))
				continue
			}
			result.Linked++
		}
	}
	return result, nil
}

// parseCreated reads what a create command printed: either a bare id, or JSON
// with an id and url. Accepting both keeps the simplest possible provider script
// a one-liner.
func parseCreated(out string) (id, url string) {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return "", ""
	}
	if strings.HasPrefix(trimmed, "{") {
		var payload struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			return strings.TrimSpace(payload.ID), strings.TrimSpace(payload.URL)
		}
	}
	// A bare id: take the last line, so a chatty provider script still works.
	lines := strings.Split(trimmed, "\n")
	return strings.TrimSpace(lines[len(lines)-1]), ""
}

// writeRemoteID records the tracker's identity in the one frontmatter key ilk
// owns, and touches nothing else in the document.
//
// This is the fence rule applied to a document's metadata. Everything a person
// wrote — the prose, the other keys, the ordering, the comments — is left exactly
// as it was; only the owned key is rewritten. Retrofitting this after a sync has
// once overwritten somebody's reasoning is a job nobody enjoys.
func writeRemoteID(p *engine.Project, m manifest.Mirror, path, remoteID, url string) error {
	abs := p.Repo.Path(path)
	data, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	content := string(data)

	start, end, ok := frontmatterBounds(content)
	if !ok {
		return fmt.Errorf("no YAML frontmatter, so there is nowhere to record the tracker's id")
	}

	block := content[start:end]
	value := map[string]any{"id": remoteID}
	if url != "" {
		value["url"] = url
	}
	rendered, err := renderKey(m.Key, value)
	if err != nil {
		return err
	}

	updated := replaceKey(block, m.Key, rendered)
	return writeAtomic(abs, content[:start]+updated+content[end:])
}

// renderKey renders one frontmatter key and its mapping.
func renderKey(key string, value map[string]any) (string, error) {
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(map[string]any{key: value}); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// replaceKey swaps a top-level key in a frontmatter block, or appends it,
// leaving every other line untouched.
func replaceKey(block, key, rendered string) string {
	lines := strings.Split(strings.TrimRight(block, "\n"), "\n")
	prefix := key + ":"

	var out []string
	replaced := false
	skipping := false
	for _, line := range lines {
		if skipping {
			// The old value's nested lines, which are indented under the key.
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.TrimSpace(line) == "" {
				continue
			}
			skipping = false
		}
		if !replaced && (line == prefix || strings.HasPrefix(line, prefix+" ")) {
			out = append(out, strings.Split(rendered, "\n")...)
			replaced = true
			skipping = true
			continue
		}
		out = append(out, line)
	}
	if !replaced {
		out = append(out, strings.Split(rendered, "\n")...)
	}
	return strings.Join(out, "\n") + "\n"
}

// frontmatterBounds locates the YAML block's content, excluding its delimiters.
func frontmatterBounds(content string) (start, end int, ok bool) {
	if !strings.HasPrefix(content, "---\n") {
		return 0, 0, false
	}
	start = len("---\n")
	rest := content[start:]
	offset := 0
	for _, line := range strings.Split(rest, "\n") {
		if strings.TrimSpace(line) == "---" {
			return start, start + offset, true
		}
		offset += len(line) + 1
	}
	return 0, 0, false
}

func writeAtomic(path, content string) error {
	tmp := path + ".ilk-mirror"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// frontmatter reads a document's YAML block.
func frontmatter(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)
	start, end, ok := frontmatterBounds(content)
	if !ok {
		return nil, fmt.Errorf("%s has no frontmatter", path)
	}
	var meta map[string]any
	if err := yaml.Unmarshal([]byte(content[start:end]), &meta); err != nil {
		return nil, err
	}
	if meta == nil {
		meta = map[string]any{}
	}
	return meta, nil
}

func scalar(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case time.Time:
		return t.Format("2006-01-02")
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

// renderIn renders a manifest string through the layer's template context, so a
// mirror can refer to the layer's own variables.
func renderIn(l *engine.ResolvedLayer, text string) (string, error) {
	return renderString(l, text)
}
