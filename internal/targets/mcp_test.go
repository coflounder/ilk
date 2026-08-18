package targets

import (
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/manifest"
)

var oneServer = []manifest.MCPServer{{Name: "ctx", Command: "srv"}}

// A document ilk half-understands is more dangerous than one it cannot parse:
// substituting an empty object for a wrong-shaped mcpServers would silently
// destroy whatever the user had there.
func TestMergeRefusesAnMCPServersThatIsNotAnObject(t *testing.T) {
	_, err := mergeMCPConfig(".mcp.json", `{"mcpServers": ["not-an-object"]}`, oneServer, true)
	if err == nil {
		t.Fatal("a wrong-shaped mcpServers was replaced instead of refused")
	}
	if !strings.Contains(err.Error(), "will not touch it") {
		t.Errorf("the refusal should say the file is left alone: %v", err)
	}
}

func TestMergePreservesTheUsersNumbersExactly(t *testing.T) {
	existing := `{"mcpServers": {"mine": {"command": "x", "timeout": 12345678901234567890}}}`
	got, err := mergeMCPConfig(".mcp.json", existing, oneServer, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "12345678901234567890") {
		t.Errorf("a number ilk does not own was rewritten:\n%s", got)
	}
	if !strings.Contains(got, `"ctx"`) {
		t.Errorf("ilk's own entry is missing:\n%s", got)
	}
}
