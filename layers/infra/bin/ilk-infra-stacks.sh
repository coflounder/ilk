#!/bin/sh
# Every directory in the infrastructure group is a stack, and a stack is a
# directory with a project file: a Pulumi.yaml, or Terraform's .tf files.
#
# A directory without one is either a stack somebody stopped halfway through or
# something that does not belong here. Both matter, and neither is visible from a
# preview: the tool simply never looks there, so the failure mode is silence
# rather than an error.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

group=${ILK_VAR_GROUP:-infra}

# Nothing to say about a group nobody has created yet.
[ -d "$group" ] || exit 0

# A project file is what makes a directory a stack to its tool.
is_stack() {
	[ -f "$1/Pulumi.yaml" ] || [ -f "$1/Pulumi.yml" ] && return 0
	for tf in "$1"/*.tf; do
		[ -f "$tf" ] && return 0
	done
	return 1
}

status=0
for dir in "$group"/*/; do
	# The glob stays literal when it matches nothing.
	[ -d "$dir" ] || continue
	stack=${dir%/}
	if ! is_stack "$stack"; then
		echo "$stack: no Pulumi.yaml and no .tf files, so no preview knows this directory exists"
		status=1
	fi
done

exit "$status"
