package targets

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coflounder/ilk/internal/manifest"
)

// Several agents read the same MCP configuration shape — a JSON document with an
// `mcpServers` object keyed by server name. `.mcp.json` (Claude Code) and
// `.cursor/mcp.json` (Cursor) are both projections of the layers' `mcp:`
// declarations through the one merge below.
//
// Every entry ilk writes says the same thing: `ilk mcp run <name>`. The real
// command lives in the layer manifest and is resolved when the agent starts the
// server, so editing a layer never rewrites agent config, credentials named by
// requires_env stay in the environment instead of a committed file, and — as
// with `ilk hook run` in settings.json — ilk's entries are recognisable by
// their command string, with no marker keys in a schema ilk does not own.

// mcpArtifact produces the co-owned config file for one agent.
func mcpArtifact(path string, in Input) (Artifact, error) {
	servers, err := in.AllMCP()
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{
		Path: path,
		Mode: manifest.ModeMerge,
		Merge: func(existing string, adopt bool) (string, error) {
			return mergeMCPConfig(path, existing, servers, adopt)
		},
	}, nil
}

// mergeMCPConfig inserts or removes ilk's server entries in an mcpServers-shaped
// document, preserving everything else. The file belongs to the user; ilk only
// co-owns it, so it is vacated on removal, never deleted.
func mergeMCPConfig(path, existing string, servers []manifest.MCPServer, adopt bool) (string, error) {
	doc := map[string]any{}
	if strings.TrimSpace(existing) != "" {
		// UseNumber keeps the user's numbers as written; float64 round-tripping
		// would silently rewrite them elsewhere in the document.
		dec := json.NewDecoder(strings.NewReader(existing))
		dec.UseNumber()
		if err := dec.Decode(&doc); err != nil {
			return "", fmt.Errorf("%s is not valid JSON, so ilk will not touch it: %w", path, err)
		}
	}

	var entries map[string]any
	if raw, ok := doc["mcpServers"]; ok {
		entries, ok = raw.(map[string]any)
		if !ok {
			return "", fmt.Errorf("%s has an mcpServers that is not an object, so ilk will not touch it", path)
		}
	}
	if entries == nil {
		entries = map[string]any{}
	}

	// Always strip ilk's own entries first, so this function is idempotent and
	// removal is just "strip, then do not re-add".
	for name, raw := range entries {
		if mcpEntryIsIlk(raw) {
			delete(entries, name)
		}
	}

	if adopt {
		for _, s := range servers {
			entries[s.Name] = map[string]any{
				"command": "ilk",
				"args":    []any{"mcp", "run", s.Name},
			}
		}
	}

	if len(entries) == 0 {
		delete(doc, "mcpServers")
	} else {
		doc["mcpServers"] = entries
	}

	if len(doc) == 0 {
		return "", nil
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out) + "\n", nil
}

// mcpEntryIsIlk reports whether a server entry was written by ilk.
func mcpEntryIsIlk(raw any) bool {
	entry, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	cmd, _ := entry["command"].(string)
	if strings.TrimSpace(cmd) != "ilk" {
		return false
	}
	args, ok := entry["args"].([]any)
	return ok && len(args) >= 2 && args[0] == "mcp" && args[1] == "run"
}
