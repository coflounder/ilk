package manifest

import (
	"strings"
	"testing"
)

func TestRequiresEnvMayBeATemplate(t *testing.T) {
	const layer = `id: test/infra
version: 0.1.0
summary: Credential fixture.
variables:
  credentials:
    default: PULUMI_ACCESS_TOKEN
    description: The variable the preview reads.
checks:
  - id: infra.preview
    run: "true"
    requires_env: [%s]
    fix: Log in.
`
	if _, err := Parse([]byte(strings.Replace(layer, "%s", `"{{ .Vars.credentials }}"`, 1)), "test"); err != nil {
		t.Fatalf("a templated entry is checked once it renders, not at parse time: %v", err)
	}
	if _, err := Parse([]byte(strings.Replace(layer, "%s", "lower_case", 1)), "test"); err == nil {
		t.Fatal("a literal entry that is not an environment variable name must still be refused")
	}
}
