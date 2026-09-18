package targets

import (
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/pelletier/go-toml/v2"
	"strings"
	"testing"
)

func TestCodexMCPPreservesUserTOMLAndRemovesOnlyItsRegion(t *testing.T) {
	original := "# Keep this comment\nmodel = \"chosen-model\"\n\n[mcp_servers.mine]\ncommand = \"custom\"\n"
	got, err := mergeCodexMCP(original, oneServer, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, original) {
		t.Fatalf("rewrote user content: %s", got)
	}
	var config map[string]any
	if err := toml.Unmarshal([]byte(got), &config); err != nil {
		t.Fatal(err)
	}
	servers := config["mcp_servers"].(map[string]any)
	if len(servers) != 2 {
		t.Fatalf("missing server: %v", servers)
	}
	again, err := mergeCodexMCP(got, oneServer, true)
	if err != nil || again != got {
		t.Fatalf("not idempotent: %v", err)
	}
	removed, err := mergeCodexMCP(got, oneServer, false)
	if err != nil || removed != original {
		t.Fatalf("removal changed original: %q, %v", removed, err)
	}
}

func TestCodexMCPRefusesConflictsAndMalformedConfig(t *testing.T) {
	for _, input := range []string{"x = [", "mcp_servers = 42", "[mcp_servers.ctx]\ncommand = \"mine\"\n"} {
		if _, err := mergeCodexMCP(input, oneServer, true); err == nil {
			t.Fatalf("accepted conflicting config: %s", input)
		}
	}
}

func TestCodexAndPiGenerateNativeIntegrations(t *testing.T) {
	in := Input{Layers: []Layer{{MCP: oneServer, Hooks: []manifest.Hook{{Event: "session-start"}, {Event: "turn-end"}}}}}
	for _, target := range []Target{codex{}, piAgent{}} {
		artifacts, err := target.Artifacts(in)
		if err != nil {
			t.Fatal(err)
		}
		if len(artifacts) != 2 {
			t.Fatalf("%s: expected two artifacts", target.Name())
		}
		for _, event := range []string{"session-start", "turn-end"} {
			if !target.Supports(event) {
				t.Fatalf("%s does not support %s", target.Name(), event)
			}
		}
	}
	artifacts, _ := (codex{}).Artifacts(in)
	hooks, err := artifacts[0].Merge("", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SessionStart", "Stop", "--target codex"} {
		if !strings.Contains(hooks, want) {
			t.Fatalf("missing %s: %s", want, hooks)
		}
	}
	artifacts, _ = (piAgent{}).Artifacts(in)
	if strings.Contains(artifacts[0].Content, "__ILK_CONFIG__") {
		t.Fatal("Pi configuration placeholder was not replaced")
	}
}
