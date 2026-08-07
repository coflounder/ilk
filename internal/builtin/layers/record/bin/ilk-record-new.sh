#!/bin/sh
# Create a record document with the naming and frontmatter already correct.
#
#   ilk record new <docs|plans|log> <title>
#   ilk record log <title>
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

DOCS_DIR='{{ index .Caps "record.docs" }}'
PLANS_DIR='{{ index .Caps "record.plans" }}'
LOG_DIR='{{ index .Caps "record.log" }}'

usage() {
	cat >&2 <<EOF
usage: ilk record new <docs|plans|log> <title>   create a document
       ilk record log <title>                    append a dated entry

The record lives under:
  docs   $DOCS_DIR
  plans  $PLANS_DIR
  log    $LOG_DIR

A full path works too, if you would rather be explicit.
EOF
	exit 2
}

[ $# -ge 2 ] || usage

dir=$1
shift
title=$*

[ -n "$title" ] || usage

# Accept the short name a person would say as well as the path it resolves to,
# so moving the record into a group does not change how the command is typed.
kind=""
case "$dir" in
docs | doc | reference | "$DOCS_DIR") dir=$DOCS_DIR kind=doc ;;
plans | plan | "$PLANS_DIR") dir=$PLANS_DIR kind=doc ;;
log | logs | "$LOG_DIR") dir=$LOG_DIR kind=log ;;
*)
	echo "ilk record: no record directory called \"$dir\"" >&2
	echo "  fix: use docs, plans or log — or one of $DOCS_DIR, $PLANS_DIR, $LOG_DIR" >&2
	exit 1
	;;
esac

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
	echo "  fix: run \`ilk apply\` to create the record directories" >&2
	exit 1
fi

today=$(date +%Y-%m-%d)

case "$kind" in
log)
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
