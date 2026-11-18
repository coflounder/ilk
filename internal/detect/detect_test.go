package detect

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func traits(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Trait)
	}
	return out
}

func TestTraits(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"an empty repository shows nothing", map[string]string{"README.md": "# x\n"}, nil},
		{"a Next app is ui", map[string]string{
			"package.json": `{"private": true, "dependencies": {"next": "15", "react": "19"}}`,
		}, []string{"ui"}},
		{"an Expo app is ui", map[string]string{
			"package.json": `{"private": true, "dependencies": {"expo": "52"}}`,
		}, []string{"ui"}},
		{"a static site is ui", map[string]string{"index.html": "<!doctype html>"}, []string{"ui"}},
		{"a Worker is a service declared as infrastructure", map[string]string{
			"wrangler.jsonc": "{}", "src/index.ts": "export default {}",
		}, []string{"service", "infra"}},
		{"a Dockerfile is a service", map[string]string{"Dockerfile": "FROM scratch\n"}, []string{"service"}},
		{"an express app is a service", map[string]string{
			"package.json": `{"private": true, "dependencies": {"express": "4"}}`,
		}, []string{"service"}},
		{"a Go main that listens is a service", map[string]string{
			"go.mod":          "module x\n",
			"cmd/api/main.go": "package main\n\nimport \"net/http\"\n\nfunc main() { http.ListenAndServe(\":8080\", nil) }\n",
		}, []string{"service"}},
		{"a Go CLI is neither a service nor a library", map[string]string{
			"go.mod":          "module x\n",
			"cmd/cli/main.go": "package main\n\nfunc main() {}\n",
			"internal/a/a.go": "package a\n",
		}, nil},
		{"a Go module with no main is a library", map[string]string{
			"go.mod": "module x\n", "x.go": "// Package x does things.\npackage x\n", "x_test.go": "package x\n",
		}, []string{"library"}},
		{"a package that publishes exports is a library", map[string]string{
			"package.json": `{"name": "x", "exports": {".": "./index.js"}}`,
		}, []string{"library"}},
		{"a private package with a main is not a library", map[string]string{
			"package.json": `{"private": true, "main": "index.js"}`,
		}, nil},
		{"a React component library is ui and a library", map[string]string{
			"package.json": `{"name": "x", "main": "dist/index.js", "peerDependencies": {"react": "19"}}`,
		}, []string{"ui", "library"}},
		{"a Python project with no scripts is a library", map[string]string{
			"pyproject.toml": "[project]\nname = \"x\"\n", "src/x/__init__.py": "",
		}, []string{"library"}},
		{"a Python project with scripts is not", map[string]string{
			"pyproject.toml": "[project]\nname = \"x\"\n\n[project.scripts]\nx = \"x:main\"\n",
		}, nil},
		{"terraform is infra", map[string]string{"infra/dns/main.tf": "resource \"x\" \"y\" {}\n"}, []string{"infra"}},
		{"a Pulumi project is infra", map[string]string{"infra/dns/Pulumi.yaml": "name: dns\n"}, []string{"infra"}},
		{"dependencies and fixtures are somebody else's", map[string]string{
			"node_modules/x/index.html":                "<html>",
			"layers/infra/test/fixtures/a/Pulumi.yaml": "name: a\n",
			"vendor/y/main.tf":                         "",
		}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Traits(tree(t, tc.files))
			if !reflect.DeepEqual(traits(got), tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for _, f := range got {
				if f.Evidence == "" {
					t.Errorf("%s has no evidence to show", f.Trait)
				}
			}
		})
	}
}
