package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/config"
)

// A layer command is a real command built from its manifest: ilk parses its
// arguments and flags, prints its help and examples, and refuses --json unless
// the command says it can answer in JSON.

const commandLayer = `id: test/drafter
version: 0.1.0
summary: Command fixture.
examples:
  - ilk drafter draft SPEC-x --title
commands:
  - name: draft
    summary: Record what it was given.
    args:
      - {name: document, required: true, usage: the plan}
      - {name: extra, usage: one more}
    flags:
      - {name: title, type: bool, usage: only the title}
      - {name: max-lines, type: int, default: "40", usage: cap the body}
      - {name: label, short: l, usage: a label}
    examples:
      - ilk drafter draft SPEC-x --title
    run: sh -c 'printf "%s\n" "$@" > args.txt; env | grep "^ILK_FLAG_" | sort > flags.txt' draft
  - name: report
    summary: Answer in JSON when asked.
    json: true
    run: sh -c 'printf "%s" "$ILK_OUTPUT" > output.txt'
`

func commandRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	layerDir := filepath.Join(root, "layer")
	mustMkdir(t, layerDir)
	mustWrite(t, filepath.Join(layerDir, "layer.yaml"), commandLayer)
	mustMkdir(t, filepath.Join(root, ".ilk"))
	cfg := config.Default()
	cfg.Agents = nil
	cfg.Set(config.LayerRef{ID: "test/drafter", Source: layerDir})
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLayerCommandFlagsReachTheScriptAsEnvironment(t *testing.T) {
	root := commandRepo(t)
	code, _, stderr := invoke(t, "-C", root, "drafter", "draft", "SPEC-x", "--title", "--label", "two words")
	if code != exitOK {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if got := readFile(t, filepath.Join(root, "args.txt")); got != "SPEC-x\n" {
		t.Errorf("arguments %q: flags should not reach the script as words", got)
	}
	want := "ILK_FLAG_LABEL=two words\nILK_FLAG_MAX_LINES=40\nILK_FLAG_TITLE=1\n"
	if got := readFile(t, filepath.Join(root, "flags.txt")); got != want {
		t.Errorf("flags reached the script as\n%s\nwant\n%s", got, want)
	}

	code, _, stderr = invoke(t, "-C", root, "drafter", "draft", "SPEC-x", "-l", "x", "--max-lines", "5")
	if code != exitOK {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	want = "ILK_FLAG_LABEL=x\nILK_FLAG_MAX_LINES=5\nILK_FLAG_TITLE=0\n"
	if got := readFile(t, filepath.Join(root, "flags.txt")); got != want {
		t.Errorf("flags reached the script as\n%s\nwant\n%s", got, want)
	}
}

func TestLayerCommandArgumentAndFlagMistakesExitTwo(t *testing.T) {
	root := commandRepo(t)
	for _, args := range [][]string{
		{"drafter", "draft"},                             // missing <document>
		{"drafter", "draft", "a", "b", "c"},              // one too many
		{"drafter", "draft", "a", "--nope"},              // undeclared flag
		{"drafter", "draft", "a", "--max-lines", "lots"}, // not an int
		{"drafter", "report", "unexpected"},              // declares no args
	} {
		code, _, stderr := invoke(t, append([]string{"-C", root}, args...)...)
		if code != exitUsage {
			t.Errorf("%v: exit %d, want %d\n%s", args, code, exitUsage, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "args.txt")); err == nil {
		t.Error("the script ran despite a usage error")
	}
}

func TestJSONIsRefusedUnlessTheCommandDeclaresIt(t *testing.T) {
	root := commandRepo(t)
	code, _, stderr := invoke(t, "-C", root, "drafter", "draft", "SPEC-x", "--json")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d\n%s", code, exitUsage, stderr)
	}
	if got := jsonErrorCode(t, stderr); got != "json_unsupported" {
		t.Errorf("code %q, want json_unsupported", got)
	}
	if _, err := os.Stat(filepath.Join(root, "args.txt")); err == nil {
		t.Error("the script ran although --json was refused")
	}

	code, _, stderr = invoke(t, "-C", root, "drafter", "report", "--json")
	if code != exitOK {
		t.Fatalf("a command with json: true should take --json: exit %d\n%s", code, stderr)
	}
	if got := readFile(t, filepath.Join(root, "output.txt")); got != "json" {
		t.Errorf("ILK_OUTPUT = %q, want json", got)
	}
}

func TestLayerCommandHelpShowsUsageFlagsAndExamples(t *testing.T) {
	root := commandRepo(t)
	code, stdout, stderr := invoke(t, "-C", root, "drafter", "draft", "--help")
	if code != exitOK {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	for _, want := range []string{
		"ilk drafter draft <document> [extra] [flags]",
		"--max-lines int",
		"(default 40)",
		"-l, --label",
		"Examples:\n  ilk drafter draft SPEC-x --title",
		"Provided by the test/drafter layer.",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help is missing %q:\n%s", want, stdout)
		}
	}
}

func TestInfoShowsTheLayersExamples(t *testing.T) {
	root := commandRepo(t)
	layerDir := filepath.Join(root, "layer")
	code, stdout, stderr := invoke(t, "-C", root, "info", layerDir)
	if code != exitOK {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "Examples\n  ilk drafter draft SPEC-x --title") {
		t.Errorf("info does not show the layer's examples:\n%s", stdout)
	}
	if !strings.Contains(stdout, "ilk drafter draft <document> [extra]") {
		t.Errorf("info does not show a command's usage:\n%s", stdout)
	}

	code, stdout, stderr = invoke(t, "-C", root, "info", layerDir, "--json")
	if code != exitOK {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	var got struct {
		Examples []string `json:"examples"`
		Commands []struct {
			Name  string `json:"name"`
			JSON  bool   `json:"json"`
			Flags []struct {
				Name string `json:"name"`
			} `json:"flags"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Examples) != 1 || got.Examples[0] != "ilk drafter draft SPEC-x --title" {
		t.Errorf("examples in JSON: %v", got.Examples)
	}
	if len(got.Commands) != 2 || len(got.Commands[0].Flags) != 3 || !got.Commands[1].JSON {
		t.Errorf("commands in JSON: %+v", got.Commands)
	}
}

func TestLayerValidateRefusesAnUnprefixedCheckID(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "layer.yaml"), `id: test/drafter
version: 0.1.0
summary: Fixture.
checks:
  - id: drafts.ready
    run: "true"
    fix: Do it.
`)
	code, _, stderr := invoke(t, "layer", "validate", dir)
	if code == exitOK {
		t.Fatal("a check id without the layer's name should not validate")
	}
	if !strings.Contains(stderr, "drafter.ready") {
		t.Errorf("the error should suggest the prefixed id: %s", stderr)
	}
}
