package targets

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/coflounder/ilk/internal/manifest"
)

//go:embed pi/extension.ts
var piExtension string

//go:embed pi/mcp-client.py
var piMCPClient string

type piAgent struct{}

func (piAgent) Name() string { return "pi" }
func (piAgent) Description() string {
	return "Pi — native lifecycle extension and session-scoped MCP connections"
}
func (piAgent) Supports(event string) bool { return event == "session-start" || event == "turn-end" }

func (t piAgent) Artifacts(in Input) ([]Artifact, error) {
	servers, err := in.AllMCP()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(servers))
	for _, s := range servers {
		names = append(names, s.Name)
	}
	events := []string{}
	for _, e := range manifest.Events {
		if t.Supports(e) && len(in.AllHooks(e)) > 0 {
			events = append(events, e)
		}
	}
	if len(names) == 0 && len(events) == 0 {
		return nil, nil
	}
	config, _ := json.Marshal(map[string]any{"servers": names, "events": events})
	return []Artifact{
		{Path: ".pi/extensions/ilk/index.ts", Mode: manifest.ModeManaged, Content: strings.Replace(piExtension, "__ILK_CONFIG__", string(config), 1)},
		{Path: ".pi/extensions/ilk/mcp-client.py", Mode: manifest.ModeManaged, Content: piMCPClient},
	}, nil
}
