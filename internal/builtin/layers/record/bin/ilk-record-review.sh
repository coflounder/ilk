#!/bin/sh
# Show what changed under a document's `covers:` paths since it was last read,
# then record that you have looked.
#
#   ilk record review <file> [--show]
#
# Acknowledging a document should be informed. A check that can be silenced by
# bumping a date teaches people to bump the date; one that shows you the commits
# you are signing off on does not.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

usage() {
	cat >&2 <<'EOF'
usage: ilk record review <file> [--show]

Prints the commits that touched the document's `covers:` paths since it was last
reviewed, then updates its `updated:` field to today.

  --show   print what changed and stop, without recording the review
EOF
	exit 2
}

file=""
show_only=0
for arg in "$@"; do
	case "$arg" in
	--show)
		show_only=1
		;;
	-h | --help)
		usage
		;;
	-*)
		echo "ilk record review: unknown option $arg" >&2
		usage
		;;
	*)
		if [ -n "$file" ]; then
			usage
		fi
		file=$arg
		;;
	esac
done

[ -n "$file" ] || usage

if [ ! -f "$file" ]; then
	echo "ilk record review: no such file: $file" >&2
	exit 1
fi

# Pull the `covers:` entries out of the frontmatter. Both YAML shapes are
# accepted: a flow list on one line, and a block list beneath the key.
covers=$(awk '
	NR == 1 && $0 != "---" { exit }
	NR == 1 { next }
	$0 == "---" { exit }
	/^covers:[[:space:]]*\[/ {
		line = $0
		sub(/^covers:[[:space:]]*\[/, "", line)
		sub(/\].*$/, "", line)
		gsub(/["'"'"']/, "", line)
		n = split(line, parts, /[[:space:]]*,[[:space:]]*/)
		for (i = 1; i <= n; i++) {
			gsub(/^[[:space:]]+|[[:space:]]+$/, "", parts[i])
			if (parts[i] != "") print parts[i]
		}
		next
	}
	/^covers:[[:space:]]*$/ { inlist = 1; next }
	inlist && /^[[:space:]]*-[[:space:]]*/ {
		item = $0
		sub(/^[[:space:]]*-[[:space:]]*/, "", item)
		gsub(/["'"'"']/, "", item)
		gsub(/^[[:space:]]+|[[:space:]]+$/, "", item)
		if (item != "") print item
		next
	}
	inlist && /^[^[:space:]-]/ { inlist = 0 }
' "$file")

if [ -z "$covers" ]; then
	cat >&2 <<EOF
ilk record review: $file declares no \`covers:\`, so there is nothing to review it against.

  fix: add the paths this document describes to its frontmatter, e.g.

    covers:
      - src/payments/**
EOF
	exit 1
fi

# Build git pathspecs, marking globs so ** behaves the way people expect.
set --
for pattern in $covers; do
	case "$pattern" in
	*[*?[]*) set -- "$@" ":(glob)$pattern" ;;
	*) set -- "$@" "$pattern" ;;
	esac
done

declared=$(sed -n 's/^updated:[[:space:]]*//p' "$file" | head -n 1 | tr -d "\"' ")

# The document's own last commit is the honest verification point: unlike the
# `updated:` field, it cannot be moved without committing something. Working in
# commit ranges rather than timestamps also keeps this portable.
doc_commit=$(git log -1 --format=%H -- "$file" 2>/dev/null || true)

echo "Reviewing $file"
echo "  covers:        $(echo "$covers" | tr '\n' ' ')"
echo "  last reviewed: ${declared:-unknown}"
echo

if [ -z "$doc_commit" ]; then
	echo "This document has never been committed, so there is no history to compare against."
	commits=""
else
	commits=$(git log --no-merges "$doc_commit..HEAD" --format='  %h  %ad  %s' --date=short -- "$@" 2>/dev/null || true)
fi

if [ -z "$commits" ]; then
	echo "Nothing has changed under those paths since then."
else
	echo "Changed since then:"
	echo "$commits"
	echo
	echo "Diffstat:"
	git diff --stat "$doc_commit" HEAD -- "$@" 2>/dev/null | sed 's/^/  /' || true
	echo
	echo "Read the document against those changes before recording the review."
fi

if [ "$show_only" -eq 1 ]; then
	echo
	echo "Not recorded. Re-run without --show once you have read it."
	exit 0
fi

today=$(date +%Y-%m-%d)

# Rewrite `updated:` inside the frontmatter block only, so a line further down the
# document that happens to start with `updated:` is left alone.
tmp="$file.ilk-review.$$"
awk -v today="$today" '
	NR == 1 && $0 == "---" { print; infm = 1; next }
	infm && $0 == "---" { print; infm = 0; next }
	infm && /^updated:/ { print "updated: " today; next }
	{ print }
' "$file" >"$tmp"

if ! grep -q "^updated: $today" "$tmp"; then
	rm -f "$tmp"
	echo "ilk record review: no \`updated:\` line in the frontmatter of $file" >&2
	exit 1
fi

mv "$tmp" "$file"
echo
echo "Recorded: updated: $today"
