package cli

import (
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/coflounder/ilk/internal/layer"
	"github.com/coflounder/ilk/internal/registry"
)

// resolveLayer loads a layer from a reference, accepting the short names that
// `ilk search` prints. A bare name that is neither built in nor a directory is
// looked up in ilk's own index, and the source it resolves to is said out loud,
// so nobody fetches code they didn't know they had named.
func resolveLayer(raw, cache string) (*layer.Loaded, error) {
	loaded, err := layer.Resolve(raw, cache)
	if err == nil || !isBareName(raw) {
		return loaded, err
	}
	entry, ok := registry.Lookup(raw)
	if !ok || entry.Builtin() {
		return nil, &cliError{Exit: exitUsage, Code: "unknown_layer", Message: err.Error()}
	}
	if !flagJSON {
		warnf("  %s %s → %s %s\n", sty.dim("resolved"), raw, entry.Source, sty.dim("(index)"))
	}
	return layer.Resolve(entry.Source, cache)
}

// isBareName is true for "blueprint" and "ilk/blueprint", and false for paths,
// git references and versions.
func isBareName(raw string) bool {
	if raw == "" || strings.ContainsAny(raw, ":@\\") || strings.HasPrefix(raw, ".") || strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "~") {
		return false
	}
	if strings.Count(raw, "/") > 1 {
		return false
	}
	if _, err := os.Stat(raw); err == nil || !errors.Is(err, fs.ErrNotExist) {
		return false
	}
	return true
}
