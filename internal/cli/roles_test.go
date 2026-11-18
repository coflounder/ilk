package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coflounder/ilk/internal/config"
)

// memoryLayer writes a layer that fills the memory role and owns one file.
func memoryLayer(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, "src", name)
	mustWrite(t, filepath.Join(dir, "layer.yaml"), fmt.Sprintf(`id: acme/%s
version: 0.1.0
summary: Memory, the %s way.
provides: [role.memory]
files:
  - src: files/%s.md
    dest: %s.md
    mode: managed
`, name, name, name, name))
	mustWrite(t, filepath.Join(dir, "files", name+".md"), name+"\n")
	return dir
}

func emptyRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, ".ilk"))
	cfg := config.Default()
	cfg.Agents = nil
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func layersIn(t *testing.T, root string) []string {
	t.Helper()
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, l := range cfg.Layers {
		ids = append(ids, l.ID)
	}
	return ids
}

func TestASecondLayerForARoleIsRefusedAndReplaceSwapsIt(t *testing.T) {
	root := emptyRepo(t)
	mem, wiki := memoryLayer(t, root, "claude-mem"), memoryLayer(t, root, "llm-wiki")
	if code, _, stderr := invoke(t, "-C", root, "add", mem, "--yes"); code != exitOK {
		t.Fatalf("first memory layer: exit %d\n%s", code, stderr)
	}

	code, _, stderr := invoke(t, "-C", root, "--json", "add", wiki)
	if code != exitFailed {
		t.Fatalf("a second memory layer: exit %d, want %d\n%s", code, exitFailed, stderr)
	}
	var got struct {
		Error struct{ Code, Message, Fix string } `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &got); err != nil {
		t.Fatalf("%v\n%s", err, stderr)
	}
	if got.Error.Code != "layer_conflict" || !strings.Contains(got.Error.Message, "memory role") || !strings.Contains(got.Error.Fix, "--replace") {
		t.Fatalf("the refusal should name the role and offer --replace: %+v", got.Error)
	}
	if ids := layersIn(t, root); len(ids) != 1 || ids[0] != "acme/claude-mem" {
		t.Fatalf("a refused add changed the config: %v", ids)
	}

	if code, _, stderr := invoke(t, "-C", root, "add", wiki, "--replace", "--yes"); code != exitOK {
		t.Fatalf("--replace: exit %d\n%s", code, stderr)
	}
	if ids := layersIn(t, root); len(ids) != 1 || ids[0] != "acme/llm-wiki" {
		t.Fatalf("--replace should leave only the new layer: %v", ids)
	}
	if _, err := os.Stat(filepath.Join(root, "claude-mem.md")); err == nil {
		t.Error("the replaced layer's file was left behind")
	}
	if _, err := os.Stat(filepath.Join(root, "llm-wiki.md")); err != nil {
		t.Error("the new layer's file was not written")
	}
}

func TestLayersThatClashWithEachOtherCannotBeAddedTogether(t *testing.T) {
	root := emptyRepo(t)
	code, _, stderr := invoke(t, "-C", root, "add", memoryLayer(t, root, "claude-mem"), memoryLayer(t, root, "llm-wiki"), "--yes", "--replace")
	if code != exitFailed || !strings.Contains(stderr, "memory role") {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if ids := layersIn(t, root); len(ids) != 0 {
		t.Fatalf("nothing should have been added: %v", ids)
	}
}

func TestABundleAddsEachMemberAndSkipsWhatIsHere(t *testing.T) {
	root := contractRepo(t) // already has test/echoer
	other := filepath.Join(root, "src", "notes")
	mustWrite(t, filepath.Join(other, "layer.yaml"), "id: test/notes\nversion: 0.1.0\nsummary: Notes.\n")
	echoer := filepath.Join(root, "layer")

	prev := expandBundle
	defer func() { expandBundle = prev }()
	expandBundle = func(ref string) ([]string, error) {
		if ref != "@both" {
			return nil, fmt.Errorf("no bundle %s", ref)
		}
		return []string{echoer, other}, nil
	}

	if code, _, stderr := invoke(t, "-C", root, "add", "@both", "--yes"); code != exitOK {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if ids := layersIn(t, root); len(ids) != 2 || ids[1] != "test/notes" {
		t.Fatalf("got %v", ids)
	}

	if code, _, _ := invoke(t, "-C", root, "add", "@both", "--var", "x=y"); code != exitUsage {
		t.Errorf("--var with several layers: exit %d, want %d", code, exitUsage)
	}
	if code, _, _ := invoke(t, "-C", root, "add", "@missing"); code != exitUsage {
		t.Errorf("an unknown bundle: exit %d, want %d", code, exitUsage)
	}
}
