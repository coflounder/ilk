package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/config"
)

// These tests hold the part of the CLI that scripts, CI and agents depend on:
// exit codes, what happens when nobody can answer a prompt, errors under --json,
// and which arguments reach a layer's own command. Each one is a bug that let a
// caller believe something happened when it had not.

const contractLayer = `id: test/echoer
version: 0.1.0
summary: Contract fixture.
files:
  - src: files/owned.md
    dest: owned.md
    mode: managed
commands:
  - name: args
    summary: Record the arguments and output mode it was given.
    run: sh -c 'printf "%s\n" "$@" > args.txt; printf "%s" "$ILK_OUTPUT" > output.txt' args
`

func contractRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	layerDir := filepath.Join(root, "layer")
	mustMkdir(t, filepath.Join(layerDir, "files"))
	mustWrite(t, filepath.Join(layerDir, "layer.yaml"), contractLayer)
	mustWrite(t, filepath.Join(layerDir, "files", "owned.md"), "owned\n")
	mustMkdir(t, filepath.Join(root, ".ilk"))
	cfg := config.Default()
	cfg.Targets = nil
	cfg.Set(config.LayerRef{ID: "test/echoer", Source: layerDir})
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	return root
}

// invoke runs the whole CLI as a process would, with stdin empty and not a
// terminal, which is what CI and agent harnesses give it.
func invoke(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	empty, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Close()

	var o, e bytes.Buffer
	prevOut, prevErr, prevIn, prevTTY := out, errOut, os.Stdin, stdinIsTerminal
	prevJSON, prevDir := flagJSON, flagDir
	out, errOut, os.Stdin = &o, &e, empty
	stdinIsTerminal = func() bool { return false }
	defer func() {
		out, errOut, os.Stdin, stdinIsTerminal = prevOut, prevErr, prevIn, prevTTY
		flagJSON, flagDir = prevJSON, prevDir
	}()
	code = run(args)
	return code, o.String(), e.String()
}

func TestPromptWithNobodyToAnswerExitsThreeAndChangesNothing(t *testing.T) {
	root := contractRepo(t)
	code, _, stderr := invoke(t, "-C", root, "apply")
	if code != exitConfirmationRequired {
		t.Fatalf("exit %d, want %d: declining silently with 0 lets a script believe it applied\n%s", code, exitConfirmationRequired, stderr)
	}
	if !strings.Contains(stderr, "--yes") {
		t.Errorf("the error should say how to proceed: %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "owned.md")); err == nil {
		t.Error("a change was written without confirmation")
	}

	code, _, stderr = invoke(t, "-C", root, "apply", "--yes")
	if code != exitOK {
		t.Fatalf("apply --yes: exit %d\n%s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "owned.md")); err != nil {
		t.Error("apply --yes did not write the layer's file")
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	for _, args := range [][]string{
		{"no-such-command"},
		{"add"},
		{"check", "--no-such-flag"},
		{"init", "unexpected-argument"},
	} {
		code, _, stderr := invoke(t, args...)
		if code != exitUsage {
			t.Errorf("ilk %s: exit %d, want %d\n%s", strings.Join(args, " "), code, exitUsage, stderr)
		}
	}
}

func TestErrorsUnderJSONAreJSON(t *testing.T) {
	root := contractRepo(t)
	for _, tc := range []struct {
		args []string
		code string
		exit int
	}{
		{[]string{"-C", root, "add", "--json"}, "usage", exitUsage},
		{[]string{"-C", root, "--json", "info", "no-such-layer-anywhere"}, "failed", exitFailed},
	} {
		code, _, stderr := invoke(t, tc.args...)
		if code != tc.exit {
			t.Errorf("%v: exit %d, want %d", tc.args, code, tc.exit)
		}
		var got struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Exit    int    `json:"exit"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(stderr), &got); err != nil {
			t.Fatalf("%v: stderr is not one JSON object: %v\n%s", tc.args, err, stderr)
		}
		if got.Error.Code != tc.code || got.Error.Exit != tc.exit || got.Error.Message == "" {
			t.Errorf("%v: got %+v", tc.args, got.Error)
		}
	}
}

func TestLayerCommandsDoNotReceiveGlobalFlags(t *testing.T) {
	root := contractRepo(t)
	code, _, stderr := invoke(t, "-C", root, "echoer", "args", "Payments overview", "--json", "--dir", root, "--", "--json")
	if code != exitOK {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(root, "args.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "Payments overview\n--json\n"; got != want {
		t.Errorf("script received %q, want %q: ilk's own flags leaked into its arguments", got, want)
	}
	mode, err := os.ReadFile(filepath.Join(root, "output.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(mode) != "json" {
		t.Errorf("ILK_OUTPUT = %q, want json", mode)
	}
}

func TestLayerCommandsAreFoundThroughDashC(t *testing.T) {
	root := contractRepo(t)
	code, _, stderr := invoke(t, "-C", root, "echoer", "args", "x")
	if code != exitOK {
		t.Fatalf("`ilk -C <repo> <layer> <command>` should find the other repository's layer commands: exit %d\n%s", code, stderr)
	}
}
