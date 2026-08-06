// Package version carries the build identity, set by the linker at release time.
package version

import "runtime/debug"

// Version is overwritten at build time with -ldflags "-X .../version.Version=...".
var Version = "dev"

// Commit is the source revision, set the same way.
var Commit = ""

// String renders the full version for `ilk version` and generated headers.
func String() string {
	v := Version
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" && len(s.Value) >= 7 {
					return "dev+" + s.Value[:7]
				}
			}
		}
	}
	if Commit != "" && len(Commit) >= 7 {
		return v + " (" + Commit[:7] + ")"
	}
	return v
}
