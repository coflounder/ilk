// Package detect reads a repository's files for the simple repository traits —
// ui, service, library, infra — so `ilk init` can suggest the layers declared
// for them.
//
// Detection only ever suggests; nothing is added on its word. Even so it stays
// conservative: each rule looks for a file or a dependency that means one
// thing, and a repository that matches none gets no suggestion rather than a
// guess. unattended-agents and multi-agent describe how people work in a
// repository, which no file says, so they are never detected.
package detect

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Finding is one detected trait and the file that showed it.
type Finding struct {
	Trait    string `json:"trait"`
	Evidence string `json:"evidence"`
}

// uiFrameworks are dependencies that only an application with an interface,
// or a component library for one, would declare.
var uiFrameworks = []string{
	"react", "react-dom", "react-native", "expo", "next", "vue", "nuxt",
	"svelte", "@sveltejs/kit", "@angular/core", "solid-js", "preact", "astro",
}

// serverFrameworks are Node dependencies whose only job is serving requests.
var serverFrameworks = []string{"express", "fastify", "koa", "hono", "@nestjs/core"}

// wranglerFiles configure a Cloudflare Worker: a deployed service. They are
// not infrastructure as code in the sense the infra layer means (a plan an
// agent previews and a person applies), so they count only as service.
var wranglerFiles = []string{"wrangler.toml", "wrangler.json", "wrangler.jsonc"}

// skipped are directories whose contents describe somebody else's code, or
// ilk's own state, rather than this repository's.
var skipped = map[string]bool{
	".git": true, ".ilk": true, "node_modules": true, "vendor": true,
	"dist": true, "build": true, "testdata": true, "fixtures": true,
	".venv": true, "venv": true, "__pycache__": true, "target": true,
}

// maxDepth bounds the walk. The files that say what a repository is sit near
// its root; a monorepo's packages one or two levels down.
const maxDepth = 4

// Traits reports the traits the repository at root shows, at most one finding
// per trait, in the order of manifest.Traits.
func Traits(root string) []Finding {
	s := scan(root)
	var out []Finding
	add := func(trait, evidence string) {
		for _, f := range out {
			if f.Trait == trait {
				return
			}
		}
		if evidence != "" {
			out = append(out, Finding{Trait: trait, Evidence: evidence})
		}
	}

	add("ui", s.ui())
	add("service", s.service())
	add("library", s.library())
	add("infra", s.infra())
	return out
}

type scanned struct {
	root     string
	files    []string // slash-separated, relative to root, sorted
	pkg      *packageJSON
	pkgFound bool
}

type packageJSON struct {
	Private          bool              `json:"private"`
	Main             string            `json:"main"`
	Exports          json.RawMessage   `json:"exports"`
	Dependencies     map[string]string `json:"dependencies"`
	DevDependencies  map[string]string `json:"devDependencies"`
	PeerDependencies map[string]string `json:"peerDependencies"`
}

func (p *packageJSON) depends(names []string) string {
	for _, n := range names {
		for _, deps := range []map[string]string{p.Dependencies, p.PeerDependencies, p.DevDependencies} {
			if _, ok := deps[n]; ok {
				return n
			}
		}
	}
	return ""
}

func scan(root string) *scanned {
	s := &scanned{root: root}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if skipped[d.Name()] || strings.Count(rel, "/") >= maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		s.files = append(s.files, rel)
		return nil
	})
	sort.Strings(s.files)
	if data, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		var p packageJSON
		if json.Unmarshal(data, &p) == nil {
			s.pkg, s.pkgFound = &p, true
		}
	}
	return s
}

func (s *scanned) has(rel string) bool {
	i := sort.SearchStrings(s.files, rel)
	return i < len(s.files) && s.files[i] == rel
}

// first returns the first file whose base name satisfies match.
func (s *scanned) first(match func(rel, base string) bool) string {
	for _, f := range s.files {
		if match(f, filepath.Base(f)) {
			return f
		}
	}
	return ""
}

// ui: a UI framework in package.json, or a hand-written index.html where an
// application keeps one.
func (s *scanned) ui() string {
	if s.pkgFound {
		if dep := s.pkg.depends(uiFrameworks); dep != "" {
			return "package.json depends on " + dep
		}
	}
	for _, rel := range []string{"index.html", "public/index.html", "src/index.html"} {
		if s.has(rel) {
			return rel
		}
	}
	return ""
}

// service: a Worker's wrangler config, a Dockerfile at the root, a Node server
// framework, or a Go main that listens for HTTP.
func (s *scanned) service() string {
	for _, f := range wranglerFiles {
		if s.has(f) {
			return f
		}
	}
	if s.has("Dockerfile") {
		return "Dockerfile"
	}
	if s.pkgFound {
		if dep := s.pkg.depends(serverFrameworks); dep != "" {
			return "package.json depends on " + dep
		}
	}
	for _, f := range s.goMains() {
		if fileContains(filepath.Join(s.root, f), "ListenAndServe") {
			return f + " serves HTTP"
		}
	}
	return ""
}

// library: a package.json that publishes an entry point, a Go module with no
// main package, or a Python project that declares no scripts and has no
// entry-point file.
func (s *scanned) library() string {
	if s.pkgFound && !s.pkg.Private && (s.pkg.Main != "" || len(s.pkg.Exports) > 0) {
		return "package.json publishes an entry point"
	}
	if s.has("go.mod") && len(s.goMains()) == 0 && s.first(func(rel, base string) bool {
		return strings.HasSuffix(base, ".go") && !strings.HasSuffix(base, "_test.go")
	}) != "" {
		return "go.mod with no main package"
	}
	if s.has("pyproject.toml") {
		data, _ := os.ReadFile(filepath.Join(s.root, "pyproject.toml"))
		text := string(data)
		declared := strings.Contains(text, "[project]") || strings.Contains(text, "[tool.poetry]")
		scripts := strings.Contains(text, "[project.scripts]") || strings.Contains(text, "[tool.poetry.scripts]") ||
			strings.Contains(text, "[project.gui-scripts]")
		entry := s.has("main.py") || s.has("app.py") || s.has("manage.py") ||
			s.first(func(rel, base string) bool { return base == "__main__.py" }) != ""
		if declared && !scripts && !entry {
			return "pyproject.toml with no scripts or entry point"
		}
	}
	return ""
}

// infra: Terraform files or a Pulumi project.
func (s *scanned) infra() string {
	if f := s.first(func(rel, base string) bool { return strings.HasSuffix(base, ".tf") }); f != "" {
		return f
	}
	return s.first(func(rel, base string) bool { return base == "Pulumi.yaml" || base == "Pulumi.yml" })
}

// goMains lists the Go files, outside tests, that declare package main.
func (s *scanned) goMains() []string {
	var mains []string
	for _, f := range s.files {
		if !strings.HasSuffix(f, ".go") || strings.HasSuffix(f, "_test.go") {
			continue
		}
		if packageClause(filepath.Join(s.root, f)) == "main" {
			mains = append(mains, f)
		}
	}
	return mains
}

// packageClause reads a Go file's package name.
func packageClause(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if rest, ok := strings.CutPrefix(line, "package "); ok {
			return strings.TrimSpace(strings.SplitN(rest, "//", 2)[0])
		}
	}
	return ""
}

func fileContains(path, needle string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(data), needle)
}
