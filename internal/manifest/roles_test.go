package manifest

import (
	"strings"
	"testing"
)

func TestKnownRolesParseAndMisspeltRolesDoNot(t *testing.T) {
	ok := "id: acme/wiki\nversion: 0.1.0\nsummary: A memory.\nprovides: [role.memory]\n"
	if _, err := Parse([]byte(ok), "ok"); err != nil {
		t.Fatalf("a known role should parse: %v", err)
	}
	// A typo would otherwise be a quiet way around the one-provider rule.
	bad := "id: acme/wiki\nversion: 0.1.0\nsummary: A memory.\nprovides: [role.memroy]\n"
	_, err := Parse([]byte(bad), "bad")
	if err == nil || !strings.Contains(err.Error(), "role.memory") {
		t.Fatalf("an unknown role should be refused, naming the known ones: %v", err)
	}
}
