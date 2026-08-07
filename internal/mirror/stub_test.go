package mirror

import (
	"github.com/coflounder/ilk/internal/layer"
	"github.com/coflounder/ilk/internal/manifest"
)

// stubLoaded is the smallest loaded layer a mirror can hang off. Mirrors render
// their commands through the owning layer's context; none of these tests
// exercise templating, so the manifest only needs to be well-formed.
func stubLoaded() layer.Loaded {
	return layer.Loaded{
		Manifest: &manifest.Layer{
			ID:      "test/fake",
			Version: "0.1.0",
			Summary: "A layer that exists only to own a mirror in tests.",
		},
		Source: "test",
	}
}
