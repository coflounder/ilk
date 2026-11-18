package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// The infra layer is named for its rule, not for Pulumi: Pulumi's commands are
// defaults, and a repository on Terraform changes two settings.
func TestInfraDefaultsToPulumiAndTakesTerraformAsTwoSettings(t *testing.T) {
	src, err := filepath.Abs(filepath.Join("..", "..", "layers", "infra"))
	if err != nil {
		t.Fatal(err)
	}
	root := emptyRepo(t)
	if code, _, stderr := invoke(t, "-C", root, "add", src, "--allow-exec", "--yes"); code != exitOK {
		t.Fatalf("add infra: exit %d\n%s", code, stderr)
	}
	get := func(key string) string {
		t.Helper()
		code, stdout, stderr := invoke(t, "-C", root, "config", "get", key)
		if code != exitOK {
			t.Fatalf("config get %s: exit %d\n%s", key, code, stderr)
		}
		return strings.TrimSpace(stdout)
	}
	if got := get("infra.preview.command"); !strings.HasPrefix(got, "pulumi preview") {
		t.Errorf("preview defaults to Pulumi, got %q", got)
	}
	if got := get("infra.up.command"); got != "pulumi up" {
		t.Errorf("apply defaults to Pulumi, got %q", got)
	}

	for key, value := range map[string]string{
		"infra.preview.command": "terraform plan -input=false",
		"infra.up.command":      "terraform apply",
	} {
		if code, _, stderr := invoke(t, "-C", root, "config", "set", key, value, "--yes"); code != exitOK {
			t.Fatalf("config set %s: exit %d\n%s", key, code, stderr)
		}
		if got := get(key); got != value {
			t.Errorf("%s: a declared value wins over the layer's default, got %q", key, got)
		}
	}
}
