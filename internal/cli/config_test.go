package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/config"
	"github.com/spf13/cobra"
)

const settingsLayer = `id: test/settings
version: 0.1.0
summary: Config fixture.
variables:
  greeting:
    default: hello
    enum: [hello, hi]
files:
  - inline: "{{ .Vars.greeting }}\n"
    dest: greeting.md
    mode: managed
checks:
  - id: settings.tests
    requires: test.command
    run: '{{ index .Caps "test.command" }}'
    fix: Fix the tests.
`

// settingsRepo is a repository with one layer that has a variable, a file that
// renders it, and a check that needs test.command. It is applied, so a change
// that touches no file has an empty plan.
func settingsRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	layerDir := filepath.Join(root, "layer")
	mustMkdir(t, layerDir)
	mustWrite(t, filepath.Join(layerDir, "layer.yaml"), settingsLayer)
	mustMkdir(t, filepath.Join(root, ".ilk"))
	cfg := config.Default()
	cfg.Agents = nil
	cfg.Set(config.LayerRef{ID: "test/settings", Source: layerDir})
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := invoke(t, "-C", root, "apply", "--yes"); code != exitOK {
		t.Fatalf("apply: exit %d\n%s", code, stderr)
	}
	return root
}

func loadConfig(t *testing.T, root string) *config.Config {
	t.Helper()
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func jsonErrorCode(t *testing.T, stderr string) string {
	t.Helper()
	var got struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &got); err != nil {
		t.Fatalf("stderr is not one JSON object: %v\n%s", err, stderr)
	}
	return got.Error.Code
}

func TestConfigSetGetUnsetACapability(t *testing.T) {
	root := settingsRepo(t)

	code, stdout, stderr := invoke(t, "-C", root, "config", "set", "test.command", "go test ./...")
	if code != exitOK {
		t.Fatalf("set: exit %d\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "test.command  (unset) → go test ./...") || !strings.Contains(stdout, "enables: settings.tests") {
		t.Errorf("set should show old → new and what it enables:\n%s", stdout)
	}
	if got := loadConfig(t, root).Capabilities["test.command"]; got != "go test ./..." {
		t.Errorf("config holds %q", got)
	}

	code, stdout, _ = invoke(t, "-C", root, "config", "get", "test.command")
	if code != exitOK || stdout != "go test ./...\n" {
		t.Errorf("get: exit %d, %q", code, stdout)
	}

	code, stdout, _ = invoke(t, "-C", root, "--json", "config", "list")
	var list configListDoc
	if err := json.Unmarshal([]byte(stdout), &list); code != exitOK || err != nil {
		t.Fatalf("list --json: exit %d, %v\n%s", code, err, stdout)
	}
	found := false
	for _, e := range list.Capabilities {
		if e.Key == "test.command" && e.Value == "go test ./..." && e.Set && e.Source == "config" {
			found = true
		}
	}
	if !found {
		t.Errorf("list does not report test.command as set in config: %+v", list.Capabilities)
	}

	code, stdout, stderr = invoke(t, "-C", root, "config", "unset", "test.command")
	if code != exitOK || !strings.Contains(stdout, "disables: settings.tests") {
		t.Fatalf("unset: exit %d\n%s%s", code, stdout, stderr)
	}
	if _, ok := loadConfig(t, root).Capabilities["test.command"]; ok {
		t.Error("unset left the capability in the config")
	}

	// Needed by a check but supplied by nothing: a value that is missing, not a
	// key nobody knows.
	code, _, stderr = invoke(t, "-C", root, "--json", "config", "get", "test.command")
	if code != exitFailed || jsonErrorCode(t, stderr) != "config_key_unset" {
		t.Errorf("get of a needed but unset capability: exit %d\n%s", code, stderr)
	}
	// Unsetting what is not set is not an error.
	if code, _, stderr := invoke(t, "-C", root, "config", "unset", "test.command"); code != exitOK {
		t.Errorf("second unset: exit %d\n%s", code, stderr)
	}
}

func TestConfigUnknownKeysAreUsageErrors(t *testing.T) {
	root := settingsRepo(t)
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"config", "get", "no.such-key"}, "unknown_config_key"},
		{[]string{"config", "unset", "no.such-key"}, "unknown_config_key"},
		{[]string{"config", "set", "Not_A_Key", "x"}, "invalid_config_key"},
		{[]string{"config", "get"}, "usage"},
		{[]string{"config", "set", "test.command"}, "usage"},
		{[]string{"config", "bogus"}, "usage"},
	} {
		args := append([]string{"-C", root, "--json"}, tc.args...)
		code, _, stderr := invoke(t, args...)
		if code != exitUsage {
			t.Errorf("ilk %s: exit %d, want %d\n%s", strings.Join(tc.args, " "), code, exitUsage, stderr)
			continue
		}
		if got := jsonErrorCode(t, stderr); got != tc.code {
			t.Errorf("ilk %s: code %q, want %q", strings.Join(tc.args, " "), got, tc.code)
		}
	}
}

func TestConfigSetALayerVariableReconciles(t *testing.T) {
	root := settingsRepo(t)
	before, _ := os.ReadFile(filepath.Join(root, ".ilk", "config.yaml"))

	// The new value changes a file, so it needs a yes, and nobody can give one.
	code, _, stderr := invoke(t, "-C", root, "config", "set", "settings.greeting", "hi")
	if code != exitConfirmationRequired {
		t.Fatalf("exit %d, want %d\n%s", code, exitConfirmationRequired, stderr)
	}
	if !strings.Contains(stderr, "ilk config set settings.greeting 'hi' --yes") {
		t.Errorf("the error should give the command to re-run: %q", stderr)
	}
	after, _ := os.ReadFile(filepath.Join(root, ".ilk", "config.yaml"))
	if string(after) != string(before) {
		t.Error("the config changed without confirmation")
	}

	if code, _, stderr := invoke(t, "-C", root, "config", "set", "settings.greeting", "hi", "--yes"); code != exitOK {
		t.Fatalf("set --yes: exit %d\n%s", code, stderr)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "greeting.md")); string(data) != "hi\n" {
		t.Errorf("greeting.md = %q: the repository was not reconciled", data)
	}
	if ref, _ := loadConfig(t, root).Layer("test/settings"); ref.Vars["greeting"] != "hi" {
		t.Errorf("vars = %v", ref.Vars)
	}

	// A value the layer does not allow is the caller's mistake.
	if code, _, _ := invoke(t, "-C", root, "config", "set", "settings.greeting", "yo", "--yes"); code != exitUsage {
		t.Errorf("value outside the enum: exit %d, want %d", code, exitUsage)
	}

	code, stdout, _ := invoke(t, "-C", root, "--json", "config", "unset", "settings.greeting")
	var doc configChangeDoc
	if err := json.Unmarshal([]byte(stdout), &doc); code != exitOK || err != nil || !doc.Applied || doc.Kind != "variable" {
		t.Fatalf("unset --json: exit %d, %v\n%s", code, err, stdout)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "greeting.md")); string(data) != "hello\n" {
		t.Errorf("greeting.md = %q after unset, want the default back", data)
	}
	code, stdout, _ = invoke(t, "-C", root, "config", "get", "settings.greeting")
	if code != exitOK || stdout != "hello\n" {
		t.Errorf("get after unset: exit %d, %q", code, stdout)
	}
}

// builtinCommands walks the command tree as a repository with no layers sees
// it, so every command found is one ilk ships.
func builtinCommands(t *testing.T) []*cobra.Command {
	t.Helper()
	prevDir := flagDir
	flagDir = t.TempDir()
	defer func() { flagDir = prevDir }()
	root := newRoot()

	var all []*cobra.Command
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		all = append(all, c)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	return all
}

func TestEveryBuiltinCommandHasAnExample(t *testing.T) {
	for _, c := range builtinCommands(t) {
		if c.Hidden || c.Annotations[helpTopicAnnotation] != "" {
			continue
		}
		if strings.TrimSpace(c.Example) == "" {
			t.Errorf("`%s` has no Example: add one, using flags it really has", c.CommandPath())
		}
	}
}

func TestEveryTopLevelCommandIsGrouped(t *testing.T) {
	for _, c := range builtinCommands(t)[0].Commands() {
		if c.Annotations[helpTopicAnnotation] != "" {
			continue
		}
		if c.GroupID == "" {
			t.Errorf("`ilk %s` has no help group: add it to commandGroups", c.Name())
		}
	}
}

func TestHelpExitCodesListsEveryCode(t *testing.T) {
	code, stdout, stderr := invoke(t, "help", "exit-codes")
	if code != exitOK {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	for _, c := range []int{exitOK, exitFailed, exitUsage, exitConfirmationRequired, exitPending} {
		if !regexp.MustCompile(fmt.Sprintf(`(?m)^  %d  `, c)).MatchString(stdout) {
			t.Errorf("exit code %d is not documented:\n%s", c, stdout)
		}
	}
	for _, exception := range []string{"ilk hook run turn-end", "ilk <layer> <command>"} {
		if !strings.Contains(stdout, exception) {
			t.Errorf("the %q exception is not documented", exception)
		}
	}
}

func TestRootHelpShowsGroupsAndTopics(t *testing.T) {
	code, stdout, _ := invoke(t, "-C", t.TempDir(), "--help")
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	last := -1
	for _, heading := range []string{"GET STARTED", "MANAGE LAYERS", "VERIFY", "CONFIGURE", "SHARE AND INTEGRATE", "AUTHOR LAYERS", "PLUMBING", "HELP TOPICS"} {
		i := strings.Index(stdout, "\n"+heading+"\n")
		if i < 0 || i < last {
			t.Errorf("%q missing or out of order in:\n%s", heading, stdout)
		}
		last = i
	}
	if strings.Contains(stdout, "LAYER COMMANDS") {
		t.Error("a repository with no layer commands should not show their heading")
	}
	if !strings.Contains(stdout, "help exit-codes") {
		t.Error("help topics are not listed as `help <topic>`")
	}
}

// Every error code the source can return is listed in `ilk help json`, so an
// agent can rely on that list being complete.
func TestHelpJSONListsEveryErrorCode(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]bool{}
	pattern := regexp.MustCompile(`Code:\s+"([a-z_]+)"`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range pattern.FindAllStringSubmatch(string(data), -1) {
			used[m[1]] = true
		}
	}
	_, stdout, _ := invoke(t, "help", "json")
	var missing []string
	for c := range used {
		if !regexp.MustCompile(`(?m)^  ` + c + ` `).MatchString(stdout) {
			missing = append(missing, c)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("error codes used but not documented in `ilk help json`: %v", missing)
	}
	if len(used) < 5 {
		t.Errorf("found only %d codes; the scan is probably broken", len(used))
	}
}
