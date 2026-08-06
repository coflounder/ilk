// Package render turns layer templates into repository content.
//
// The template surface is deliberately small. Layers are meant to be readable by
// whoever is deciding whether to adopt them, and a large templating DSL works
// against that.
package render

import (
	"fmt"
	"sort"
	"strings"
	"text/template"
)

// Context is everything a layer template may read.
type Context struct {
	// Repo describes the repository being scaffolded.
	Repo RepoInfo
	// Vars holds this layer's resolved variables.
	Vars map[string]string
	// Caps holds the repository's capability values (e.g. test.command).
	Caps map[string]string
	// Layers lists the ids of every adopted layer, so a layer can adapt to its
	// neighbours without depending on them.
	Layers []string
	// Ilk describes the tool itself.
	Ilk IlkInfo
}

// RepoInfo describes the target repository.
type RepoInfo struct {
	Name string
	Root string
}

// IlkInfo describes the running binary.
type IlkInfo struct {
	Version string
}

func funcs() template.FuncMap {
	return template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": func(s string) string {
			if s == "" {
				return s
			}
			return strings.ToUpper(s[:1]) + s[1:]
		},
		"trim":      strings.TrimSpace,
		"join":      strings.Join,
		"split":     strings.Split,
		"replace":   func(old, new, s string) string { return strings.ReplaceAll(s, old, new) },
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		"contains":  strings.Contains,
		"indent":    indent,
		"default": func(def, v string) string {
			if strings.TrimSpace(v) == "" {
				return def
			}
			return v
		},
		"quote":      func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` },
		"sortedKeys": sortedKeys,
	}
}

func indent(n int, s string) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// String renders a template with the given context. Missing keys are an error
// rather than an empty string: a layer that references a variable it never
// declared should fail at plan time, not produce a half-written file.
func String(name, tmpl string, ctx Context) (string, error) {
	t, err := template.New(name).Funcs(funcs()).Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	var buf strings.Builder
	if err := t.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	return buf.String(), nil
}

// Path renders a destination path. Paths are rendered with the same context as
// content so a layer can honour a variable like docs_dir in both.
func Path(tmpl string, ctx Context) (string, error) {
	out, err := String("path:"+tmpl, tmpl, ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Truthy evaluates a `when:` expression. An empty expression is true. The
// grammar is intentionally tiny: a rendered value that is non-empty and not a
// falsey word.
func Truthy(expr string, ctx Context) (bool, error) {
	if strings.TrimSpace(expr) == "" {
		return true, nil
	}
	out, err := String("when:"+expr, expr, ctx)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(out)) {
	case "", "false", "0", "no", "off", "<no value>":
		return false, nil
	default:
		return true, nil
	}
}
