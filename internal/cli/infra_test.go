package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/layer"
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

func infraSource(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("..", "..", "layers", "infra"))
	if err != nil {
		t.Fatal(err)
	}
	return src
}

// A Pulumi.yaml dropped beside .tf files makes the preview run the wrong tool,
// so on Terraform or OpenTofu the layer seeds nothing and its defaults follow.
func TestInfraOnOpenTofuSeedsNoPulumiProject(t *testing.T) {
	root := emptyRepo(t)
	mustMkdir(t, filepath.Join(root, "infra", "dns"))
	mustWrite(t, filepath.Join(root, "infra", "dns", "main.tf"), "terraform {}\n")
	if code, _, stderr := invoke(t, "-C", root, "add", infraSource(t), "--var", "tool=opentofu", "--allow-exec", "--yes"); code != exitOK {
		t.Fatalf("add infra: exit %d\n%s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "infra", "dns", "Pulumi.yaml")); !os.IsNotExist(err) {
		t.Errorf("a Pulumi.yaml was seeded into an OpenTofu stack (stat err %v)", err)
	}
	for key, want := range map[string]string{
		"infra.preview.command": "tofu plan -input=false",
		"infra.up.command":      "tofu apply",
	} {
		code, stdout, stderr := invoke(t, "-C", root, "config", "get", key)
		if code != exitOK {
			t.Fatalf("config get %s: exit %d\n%s", key, code, stderr)
		}
		if got := strings.TrimSpace(stdout); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	// The Pulumi credential is not what an OpenTofu preview waits for.
	code, stdout, _ := invoke(t, "-C", root, "--json", "check", "--only", "infra.preview")
	if strings.Contains(stdout, "PULUMI_ACCESS_TOKEN") {
		t.Errorf("an OpenTofu preview waited on Pulumi's credential (exit %d):\n%s", code, stdout)
	}
}

// "Set it to nothing" is what the layer tells a project whose preview needs no
// credential, so the command has to take it. A capability still refuses one.
func TestConfigSetTakesAnEmptyVariableButNotAnEmptyCapability(t *testing.T) {
	root := emptyRepo(t)
	if code, _, stderr := invoke(t, "-C", root, "add", infraSource(t), "--allow-exec", "--yes"); code != exitOK {
		t.Fatalf("add infra: exit %d\n%s", code, stderr)
	}
	if code, _, stderr := invoke(t, "-C", root, "config", "set", "infra.credentials", "", "--yes"); code != exitOK {
		t.Fatalf("an empty variable was refused: exit %d\n%s", code, stderr)
	}
	cfg, err := os.ReadFile(filepath.Join(root, ".ilk", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), `credentials: ""`) {
		t.Errorf("the empty value was not saved:\n%s", cfg)
	}
	if code, _, _ := invoke(t, "-C", root, "config", "set", "test.command", "", "--yes"); code != exitUsage {
		t.Errorf("an empty capability exited %d, want %d", code, exitUsage)
	}
}

// infra/ in a repository is the Terraform estate, not a layer: a bare name
// only resolves to a directory that holds a layer.yaml, and otherwise goes to
// the index.
func TestABareNameIgnoresADirectoryThatIsNotALayer(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "infra", "dns"))
	mustWrite(t, filepath.Join(root, "infra", "dns", "main.tf"), "terraform {}\n")
	mustMkdir(t, filepath.Join(root, "mine"))
	mustWrite(t, filepath.Join(root, "mine", "layer.yaml"), "id: me/mine\n")
	t.Chdir(root)

	if !isBareName("infra") {
		t.Error("infra/ with no layer.yaml shadowed the index's infra layer")
	}
	if isBareName("mine") {
		t.Error("a directory holding a layer.yaml should still be taken as that layer")
	}
	if _, err := layer.Resolve("infra", filepath.Join(root, ".ilk", "cache")); err == nil || strings.Contains(err.Error(), "layer.yaml") {
		t.Errorf("resolving infra should report an unknown layer for the index to answer, got %v", err)
	}
}

// The infra layer defaults to Pulumi, so init's suggestion for a Terraform
// estate has to carry the tool, or following it seeds the wrong project file.
func TestInitSuggestsTheInfraToolItFound(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "infra", "dns"))
	mustWrite(t, filepath.Join(root, "infra", "dns", "main.tf"), "terraform {}\n")
	code, _, stderr := invoke(t, "-C", root, "--json", "init")
	if code != exitOK {
		t.Fatalf("init: exit %d\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "ilk add infra --var tool=terraform") {
		t.Errorf("the suggestion did not carry the tool:\n%s", stderr)
	}
}

// The layer seeds one stack; the group's index has to name every one there.
func TestInfraIndexListsEveryStackOnDisk(t *testing.T) {
	root := emptyRepo(t)
	for _, stack := range []string{"dns", "auth", ".terraform"} {
		mustMkdir(t, filepath.Join(root, "infra", stack))
		mustWrite(t, filepath.Join(root, "infra", stack, "main.tf"), "terraform {}\n")
	}
	if code, _, stderr := invoke(t, "-C", root, "add", infraSource(t), "--var", "tool=terraform", "--allow-exec", "--yes"); code != exitOK {
		t.Fatalf("add infra: exit %d\n%s", code, stderr)
	}
	index, err := os.ReadFile(filepath.Join(root, "infra", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[`dns/`](dns/)", "[`auth/`](auth/)"} {
		if !strings.Contains(string(index), want) {
			t.Errorf("the index is missing %s:\n%s", want, index)
		}
	}
	if strings.Contains(string(index), ".terraform") {
		t.Errorf("tool state is not a stack, but the index lists it:\n%s", index)
	}
	// A clean repository stays clean: the index it wrote is the index it wants.
	if code, stdout, stderr := invoke(t, "-C", root, "apply", "--dry-run"); code != exitOK || strings.Contains(stdout, "README.md") {
		t.Errorf("a second plan wants to change the index (exit %d):\n%s%s", code, stdout, stderr)
	}
}
