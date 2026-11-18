package manifest

import (
	"sort"
	"strings"
)

// RolePrefix marks a capability whose meaning ilk defines rather than the layer.
const RolePrefix = "role."

// Roles are the jobs ilk names, so that layers written by different people for
// the same job say so in the same words. Two memory layers, two agent loops or
// two preview deployers in one repository would each believe it was in charge;
// naming the role is what lets ilk see that and refuse.
//
// A role is an ordinary capability — `provides: [role.memory]` — and, like every
// capability, it has one provider per repository. The list is closed so that a
// misspelt role is a manifest error rather than a quiet way around the check.
var Roles = map[string]string{
	"role.memory":     "keeps what agents learn and recalls it in later sessions",
	"role.agent-loop": "decides whether an agent keeps working after it says it is done",
	"role.preview":    "deploys a preview of each change and reports where it is",
}

// IsRole reports whether a capability name is in the role namespace.
func IsRole(capability string) bool { return strings.HasPrefix(capability, RolePrefix) }

// RoleNames lists the known roles, sorted.
func RoleNames() []string {
	names := make([]string, 0, len(Roles))
	for name := range Roles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
