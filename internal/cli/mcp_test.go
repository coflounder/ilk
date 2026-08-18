package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/config"
)

// `ilk mcp run` is what every projected agent entry invokes, so its contract —
// render the declaration, refuse on a missing credential by name, hand back the
// child's exit code — is what the projections silently rely on.

func mcpRepo(t *testing.T, manifestText string) string {
	t.Helper()
	root := t.TempDir()
	layerDir := filepath.Join(root, "layer")
	mustMkdir(t, layerDir)
	mustWrite(t, filepath.Join(layerDir, "layer.yaml"), manifestText)
	if err := os.MkdirAll(filepath.Join(root, ".ilk"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Targets = nil
	cfg.Set(config.LayerRef{ID: "test/mcp", Source: layerDir})
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func runMCP(t *testing.T, root string, args ...string) (int, error) {
	t.Helper()
	prevDir, prevCode := flagDir, exitCode
	flagDir, exitCode = root, 0
	t.Cleanup(func() { flagDir, exitCode = prevDir, prevCode })
	cmd := newMCPRunCmd()
	cmd.SetArgs(args)
	err := cmd.Execute()
	return exitCode, err
}

func TestMCPRunRefusesWithoutTheNamedCredential(t *testing.T) {
	root := mcpRepo(t, `id: test/mcp
version: 0.1.0
summary: Test.
mcp:
  - name: gated
    command: sh
    args: ["-c", "true"]
    requires_env: [ILK_TEST_ABSENT_TOKEN]
`)
	os.Unsetenv("ILK_TEST_ABSENT_TOKEN")
	_, err := runMCP(t, root, "gated")
	if err == nil {
		t.Fatal("a server missing its credential started anyway")
	}
	if !strings.Contains(err.Error(), "ILK_TEST_ABSENT_TOKEN") {
		t.Errorf("the refusal should name the missing variable: %v", err)
	}
}

func TestMCPRunPropagatesTheChildsExitCode(t *testing.T) {
	root := mcpRepo(t, `id: test/mcp
version: 0.1.0
summary: Test.
mcp:
  - name: failing
    command: sh
    args: ["-c", "exit 7"]
`)
	code, err := runMCP(t, root, "failing")
	if err != nil {
		t.Fatalf("a non-zero child exit is the child's report, not ilk's error: %v", err)
	}
	if code != 7 {
		t.Errorf("exit code %d, want the child's 7", code)
	}
}

func TestMCPRunRendersArgsAgainstTheLayersVariables(t *testing.T) {
	root := mcpRepo(t, `id: test/mcp
version: 0.1.0
summary: Test.
variables:
  code: { default: "3" }
mcp:
  - name: templated
    command: sh
    args: ["-c", "exit {{ .Vars.code }}"]
`)
	code, err := runMCP(t, root, "templated")
	if err != nil {
		t.Fatal(err)
	}
	if code != 3 {
		t.Errorf("exit code %d, want 3 — args were not rendered against the layer's variables", code)
	}
}

func TestMCPRunNamesTheListForAnUnknownServer(t *testing.T) {
	root := mcpRepo(t, `id: test/mcp
version: 0.1.0
summary: Test.
mcp:
  - name: real
    command: sh
    args: ["-c", "true"]
`)
	_, err := runMCP(t, root, "imaginary")
	if err == nil || !strings.Contains(err.Error(), "ilk mcp list") {
		t.Errorf("an unknown name should point at `ilk mcp list`: %v", err)
	}
}
