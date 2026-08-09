#!/bin/sh
# Every directory in the infrastructure group is a Pulumi stack, and a stack is a
# directory with a Pulumi.yaml.
#
# A directory without one is either a stack somebody stopped halfway through or
# something that does not belong here. Both matter, and neither is visible from a
# preview: pulumi simply never looks there, so the failure mode is silence rather
# than an error.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

group=${ILK_VAR_GROUP:-infra}

# Nothing to say about a group nobody has created yet.
[ -d "$group" ] || exit 0

status=0
for dir in "$group"/*/; do
	# The glob stays literal when it matches nothing.
	[ -d "$dir" ] || continue
	stack=${dir%/}
	if [ ! -f "$stack/Pulumi.yaml" ]; then
		echo "$stack: no Pulumi.yaml, so pulumi does not know this directory exists"
		status=1
	fi
done

exit "$status"
