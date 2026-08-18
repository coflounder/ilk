package engine

import (
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/config"
)

// The contract: a layer declares an MCP server once, in neutral form, and each
// configured agent's config file becomes a projection of it — co-owned, so a
// file the user already had is joined and vacated, never overwritten or deleted.

// mcpLayer writes a layer directory declaring one MCP server.
func mcpLayer(t *testing.T, dir, id, server string) string {
	t.Helper()
	manifestText := `id: ` + id + `
version: 0.1.0
summary: A layer declaring an MCP server.
mcp:
  - name: ` + server + `
    summary: A test server.
    command: my-mcp-server
    args: ["--stdio"]
`
	write(t, dir, "layer.yaml", manifestText)
	return dir
}

func mcpConfig(t *testing.T, root string, layers map[string]string) {
	t.Helper()
	cfg := config.Default()
	cfg.Targets = []string{"claude-code", "cursor"}
	for id, dir := range layers {
		cfg.Set(config.LayerRef{ID: id, Source: dir})
	}
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
}

func TestMCPServersProjectIntoEachAgentsConfig(t *testing.T) {
	root := fixture(t)
	layerDir := mcpLayer(t, t.TempDir(), "test/mcp", "context-server")
	mcpConfig(t, root, map[string]string{"test/mcp": layerDir})
	apply(t, root)

	for _, path := range []string{".mcp.json", ".cursor/mcp.json"} {
		got := read(t, root, path)
		if !strings.Contains(got, `"context-server"`) {
			t.Errorf("%s does not name the server:\n%s", path, got)
		}
		if !strings.Contains(got, `"ilk"`) || !strings.Contains(got, `"run"`) {
			t.Errorf("%s should point at `ilk mcp run`, not the raw command:\n%s", path, got)
		}
		if strings.Contains(got, "my-mcp-server") {
			t.Errorf("%s carries the real command — that belongs in the manifest, resolved at start time:\n%s", path, got)
		}
	}
}

func TestMCPJoinsAndVacatesAFileTheUserAlreadyHad(t *testing.T) {
	root := fixture(t)
	mine := `{
  "mcpServers": {
    "my-own": {
      "command": "something-i-configured"
    }
  }
}
`
	write(t, root, ".mcp.json", mine)

	layerDir := mcpLayer(t, t.TempDir(), "test/mcp", "context-server")
	mcpConfig(t, root, map[string]string{"test/mcp": layerDir})
	apply(t, root)

	got := read(t, root, ".mcp.json")
	if !strings.Contains(got, `"my-own"`) || !strings.Contains(got, "something-i-configured") {
		t.Fatalf("joining the file disturbed the user's own server:\n%s", got)
	}
	if !strings.Contains(got, `"context-server"`) {
		t.Fatalf("ilk's server was not added:\n%s", got)
	}

	mcpConfig(t, root, map[string]string{})
	apply(t, root)

	got = read(t, root, ".mcp.json")
	if strings.Contains(got, "context-server") {
		t.Errorf("removal left ilk's entry behind:\n%s", got)
	}
	if !strings.Contains(got, `"my-own"`) {
		t.Errorf("removal took the user's server with it:\n%s", got)
	}
}

func TestMCPRemovalDeletesOnlyTheFileIlkCreated(t *testing.T) {
	root := fixture(t)
	layerDir := mcpLayer(t, t.TempDir(), "test/mcp", "context-server")
	mcpConfig(t, root, map[string]string{"test/mcp": layerDir})
	apply(t, root)
	if !exists(root, ".mcp.json") {
		t.Fatal("setup: .mcp.json was not created")
	}

	cfg := config.Default()
	cfg.Targets = nil
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	apply(t, root)

	// The user never had these files; ilk created them, so vacating empties
	// them and empty means gone.
	if exists(root, ".mcp.json") {
		t.Error(".mcp.json left behind after remove")
	}
	if exists(root, ".cursor/mcp.json") {
		t.Error(".cursor/mcp.json left behind after remove")
	}
}

// A target with nothing to contribute must leave no trace: no file, and no
// lockfile entry for the drift check to hold against a file that was
// deliberately never written.
func TestNoDeclaredServersLeavesNoFileAndNoLockEntry(t *testing.T) {
	root := fixture(t)
	configWith(t, root) // claude-code configured, no layer declares a server
	apply(t, root)

	if exists(root, ".mcp.json") {
		t.Error(".mcp.json created with no servers to put in it")
	}
	if strings.Contains(read(t, root, ".ilk/lock.json"), ".mcp.json") {
		t.Error("the lockfile claims a file that was deliberately never written")
	}
}

func TestTwoLayersDeclaringTheSameMCPNameIsAnError(t *testing.T) {
	root := fixture(t)
	a := mcpLayer(t, t.TempDir(), "test/one", "same-name")
	b := mcpLayer(t, t.TempDir(), "test/two", "same-name")
	mcpConfig(t, root, map[string]string{"test/one": a, "test/two": b})

	p, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Plan(PlanOptions{Prune: true})
	if err == nil {
		t.Fatal("two layers declaring the same server name should refuse to plan")
	}
	if !strings.Contains(err.Error(), "same-name") {
		t.Errorf("the error should name the colliding server: %v", err)
	}
}
