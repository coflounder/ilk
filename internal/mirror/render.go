package mirror

import (
	"github.com/coflounder/ilk/internal/engine"
	"github.com/coflounder/ilk/internal/render"
)

// renderString renders a manifest value through the owning layer's context, so a
// mirror declaration can refer to the layer's own variables the way every other
// part of a manifest can.
func renderString(l *engine.ResolvedLayer, text string) (string, error) {
	if text == "" {
		return "", nil
	}
	return render.String("mirror:"+l.ID(), text, l.Ctx)
}
