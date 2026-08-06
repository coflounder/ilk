#!/bin/sh
# Create a record document with the naming and frontmatter already correct.
#
#   ilk record new <docs|plans|log> <title>
#   ilk record log <title>
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

usage() {
	cat >&2 <<'EOF'
usage: ilk record new <dir> <title>    create a document in docs/ or plans/
       ilk record log <title>          append a dated entry to log/

The directory argument is the record directory name as configured in
.ilk/config.yaml — normally docs, plans or log.
EOF
	exit 2
}

[ $# -ge 2 ] || usage

dir=$1
shift
title=$*

[ -n "$title" ] || usage

# kebab-case the title: lowercase, non-alphanumerics to dashes, squeeze, trim.
slug=$(printf '%s' "$title" |
	tr '[:upper:]' '[:lower:]' |
	sed -e 's/[^a-z0-9]\{1,\}/-/g' -e 's/^-//' -e 's/-$//')

if [ -z "$slug" ]; then
	echo "ilk record: title \"$title\" contains no usable characters" >&2
	exit 1
fi

if [ ! -d "$dir" ]; then
	echo "ilk record: no such directory: $dir" >&2
	echo "  fix: run \`ilk apply\` to create the record directories, or pass one of the configured names" >&2
	exit 1
fi

today=$(date +%Y-%m-%d)

case "$dir" in
*log*)
	file="$dir/$today-$slug.md"
	[ ! -e "$file" ] || { echo "ilk record: $file already exists" >&2; exit 1; }
	cat >"$file" <<EOF
---
date: $today
title: $title
---

# $title

<!-- What happened, and what a future reader would otherwise have to reconstruct. -->
EOF
	;;
*)
	# Documents and plans take an uppercase type prefix. Default to a generic one
	# and tell the caller to sharpen it — the grammar allows any 2-6 letter type.
	prefix=${ILK_RECORD_TYPE:-DOC}
	file="$dir/$prefix-$slug.md"
	[ ! -e "$file" ] || { echo "ilk record: $file already exists" >&2; exit 1; }
	cat >"$file" <<EOF
---
id: $(printf '%s' "$prefix-$slug" | tr '[:upper:]' '[:lower:]')
title: $title
status: draft
updated: $today
covers:
  # Paths this document describes. Staleness is measured against these, so a
  # document goes stale when its subject changes rather than on a timer.
  # Replace this with real paths — a pattern matching nothing exempts the
  # document from ever being checked.
  - src/**
---

# $title

<!-- Present tense, current state. -->
EOF
	;;
esac

echo "$file"
if [ "${prefix:-}" = "DOC" ]; then
	echo "  rename the DOC- prefix to the right type (ARCH, API, OPS, DEC, REF), or set ILK_RECORD_TYPE" >&2
fi
