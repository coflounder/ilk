package builtin

// Core lists the layers every repository has. They are not chosen: ilk adds them
// to a project whether or not .ilk/config.yaml lists them, refuses to remove
// them, and `ilk search` leaves them out because there is nothing to decide.
//
// record is the platform most other layers build on — they require
// `record.plans` or `record.docs` — and toolkit is how an agent learns to operate
// ilk at all. Making either optional only created a way to be wrong.
//
// The list lives in the binary, not in a manifest, so a layer from elsewhere
// cannot declare itself core.
var Core = []string{"ilk/record", "ilk/toolkit"}

// IsCore reports whether a layer id is in the core tier.
func IsCore(id string) bool {
	for _, c := range Core {
		if c == id {
			return true
		}
	}
	return false
}

// Default lists the layers `ilk init` adds unless told otherwise. Unlike the
// core tier they are ordinary layers: each can be removed, and init skips one
// whose requirements the repository cannot meet yet.
var Default = []string{"ilk/gates", "ilk/secrets", "ilk/compound-lessons"}
