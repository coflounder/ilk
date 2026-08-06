// Package builtin embeds the layers that ship inside the ilk binary.
//
// These exist so that `ilk init` produces a repository that is immediately more
// useful than the one before it, with no registry, no network, and no choices to
// make. Everything past them is opt-in.
package builtin

import "embed"

// FS holds the built-in layer trees, rooted at "layers/".
//
//go:embed all:layers
var FS embed.FS
