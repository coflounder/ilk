#!/bin/sh
# Preview the stacks this change touched.
#
#   ilk-pulumi-preview.sh '<preview command>'
#
# The preview command comes from the repository, as `infra.preview.command`. It
# runs with a stack directory as its working directory, so it must select its own
# stack and must not prompt.
#
# There is no companion script for `up`, and there is not going to be one. The
# safe half of this loop is automatic; the half that changes the world stays a
# decision a person makes on purpose. An agent that can apply infrastructure can
# delete a production database while reasoning about a typo.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

group=${ILK_VAR_GROUP:-infra}
base=${ILK_VAR_BASE_REF:-origin/main}

if [ $# -lt 1 ] || [ -z "$1" ]; then
	echo "no preview command: set infra.preview.command under capabilities: in .ilk/config.yaml" >&2
	echo "  e.g. infra.preview.command: pulumi preview --diff --non-interactive --stack prod" >&2
	exit 2
fi
preview=$1

# Nothing to say about a group nobody has created yet.
[ -d "$group" ] || exit 0

# Which paths this change touched. The union of three questions, because any one
# of them alone has a blind spot: the branch may not be pushed, the work may not
# be committed, and a brand new stack is untracked until it is added.
changed_paths() {
	if ! git rev-parse --git-dir >/dev/null 2>&1; then
		# No history, so "what this change touched" is unanswerable. Preview
		# everything rather than quietly previewing nothing — an empty result
		# here would read exactly like a clean one.
		for dir in "$group"/*/; do
			[ -d "$dir" ] && echo "${dir%/}/."
		done
		return 0
	fi
	if git rev-parse --verify --quiet "$base" >/dev/null 2>&1; then
		merge_base=$(git merge-base "$base" HEAD 2>/dev/null || true)
		if [ -n "$merge_base" ]; then
			git diff --name-only "$merge_base" -- "$group"
		fi
	fi
	git diff --name-only HEAD -- "$group" 2>/dev/null || true
	git ls-files --others --exclude-standard -- "$group"
}

# Reduce touched paths to the stack directories that contain them.
stacks=$(mktemp)
trap 'rm -f "$stacks"' EXIT
changed_paths | awk -v group="$group" '
{
	path = $0
	sub("^" group "/", "", path)
	i = index(path, "/")
	if (i > 1) print group "/" substr(path, 1, i - 1)
}
' | sort -u >"$stacks"

if [ ! -s "$stacks" ]; then
	echo "No stack under $group/ was touched by this change; nothing to preview."
	exit 0
fi

# Read from a file rather than a pipe: a pipeline puts the loop in a subshell, and
# the failure of the last stack would be the only one that survived it.
status=0
while read -r stack; do
	if [ ! -f "$stack/Pulumi.yaml" ]; then
		echo "$stack: no Pulumi.yaml, so there is nothing to preview here (see infra.stack-shape)"
		status=1
		continue
	fi
	echo "── $stack ──"
	if ! (cd "$stack" && sh -c "$preview"); then
		status=1
	fi
done <"$stacks"

if [ "$status" -eq 0 ]; then
	echo
	echo "Preview is clean. That makes the change ready for a person to apply — it does"
	echo "not make it applied, and applying it is not your call."
fi

exit "$status"
