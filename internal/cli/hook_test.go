package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeHooksEachReceivePayloadAndStopBlocks(t *testing.T) {
	root := mcpRepo(t, `id: test/mcp
version: 0.1.0
summary: Hooks.
hooks:
  - event: turn-end
    name: first
    run: cat > first.json
  - event: turn-end
    name: second
    blocking: true
    run: cat > second.json; echo publish-a-summary >&2; exit 2
`)
	payload := `{"session_id":"abc","stop_hook_active":false}`
	input, err := os.CreateTemp(t.TempDir(), "payload")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if _, err := input.WriteString(payload); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	oldIn, oldDir, oldCode := os.Stdin, flagDir, exitCode
	os.Stdin, flagDir, exitCode = input, root, 0
	defer func() { os.Stdin, flagDir, exitCode = oldIn, oldDir, oldCode }()
	cmd := newHookRunCmd()
	cmd.SetArgs([]string{"turn-end"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if exitCode != 2 {
		t.Fatalf("Stop must receive exit 2, got %d", exitCode)
	}
	for _, name := range []string{"first.json", "second.json"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(data) != payload {
			t.Fatalf("%s did not receive the full payload: %q, %v", name, data, err)
		}
	}
}

func TestGitHookKeepsNormalFailureCode(t *testing.T) {
	root := mcpRepo(t, `id: test/mcp
version: 0.1.0
summary: Hooks.
hooks:
  - event: pre-commit
    blocking: true
    run: exit 7
`)
	oldDir, oldCode := flagDir, exitCode
	flagDir, exitCode = root, 0
	defer func() { flagDir, exitCode = oldDir, oldCode }()
	cmd := newHookRunCmd()
	cmd.SetArgs([]string{"pre-commit"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if exitCode != 1 {
		t.Fatalf("git hook failure code = %d, want 1", exitCode)
	}
}
