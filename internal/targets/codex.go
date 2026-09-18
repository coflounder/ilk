package targets

import (
	"fmt"
	"strings"

	"github.com/coflounder/ilk/internal/fence"
	"github.com/coflounder/ilk/internal/manifest"
	"github.com/pelletier/go-toml/v2"
)

type codex struct{}

func (codex) Name() string { return "codex" }
func (codex) Description() string {
	return "Codex — native hooks and MCP servers; reads AGENTS.md and skills directly"
}
func (codex) Supports(event string) bool { return (claudeCode{}).Supports(event) }

func (t codex) Artifacts(in Input) ([]Artifact, error) {
	servers, err := in.AllMCP()
	if err != nil {
		return nil, err
	}
	var events []string
	for _, event := range manifest.Events {
		if t.Supports(event) && len(in.AllHooks(event)) > 0 {
			events = append(events, event)
		}
	}
	return []Artifact{
		{Path: ".codex/hooks.json", Mode: manifest.ModeMerge, Merge: func(existing string, adopt bool) (string, error) {
			return mergeHookSettings(".codex/hooks.json", "codex", existing, events, adopt)
		}},
		{Path: ".codex/config.toml", Mode: manifest.ModeMerge, Merge: func(existing string, adopt bool) (string, error) {
			return mergeCodexMCP(existing, servers, adopt)
		}},
	}, nil
}

// TOML has comments, so keep all user bytes and replace only our fenced region.
// Parse before and after: duplicate tables or a conflicting user server must be
// refused rather than leaving a configuration Codex cannot load.
func mergeCodexMCP(existing string, servers []manifest.MCPServer, adopt bool) (string, error) {
	style := fence.StyleFor("config.toml")
	marker := fence.Marker{Layer: "target:codex", Region: "mcp"}
	clean, _, err := fence.Remove(existing, style, marker)
	if err != nil {
		return "", err
	}
	var doc map[string]any
	if err := toml.Unmarshal([]byte(clean), &doc); err != nil {
		return "", fmt.Errorf(".codex/config.toml is invalid; ilk will not touch it: %w", err)
	}
	if !adopt || len(servers) == 0 {
		return clean, nil
	}
	existingServers, ok := doc["mcp_servers"].(map[string]any)
	if _, exists := doc["mcp_servers"]; exists && !ok {
		return "", fmt.Errorf(".codex/config.toml mcp_servers must be a table")
	}
	var body strings.Builder
	for _, server := range servers {
		if _, exists := existingServers[server.Name]; exists {
			return "", fmt.Errorf(".codex/config.toml already has a user-owned MCP server %q; rename it or the layer server", server.Name)
		}
		fmt.Fprintf(&body, "[mcp_servers.%s]\ncommand = \"ilk\"\nargs = [\"mcp\", \"run\", %s]\nstartup_timeout_sec = 60\n\n", jsonString(server.Name), jsonString(server.Name))
	}
	merged, err := fence.Upsert(clean, style, marker, strings.TrimSpace(body.String()))
	if err != nil {
		return "", err
	}
	if err := toml.Unmarshal([]byte(merged), &doc); err != nil {
		return "", fmt.Errorf("generated Codex config would be invalid: %w", err)
	}
	return merged, nil
}
