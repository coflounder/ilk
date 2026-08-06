#!/bin/sh
# Move a superseded document into the archive, recording what replaced it.
#
#   ilk archive it <file> [replacement-id]
#
# Deleting a document loses the reasoning that makes its replacement legible.
# Archiving keeps it and marks it as no longer true.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

archive_dir=${ILK_VAR_ARCHIVE_DIR:-.archived}

if [ $# -lt 1 ]; then
	echo "usage: ilk archive it <file> [replacement-id]" >&2
	exit 2
fi

file=$1
replacement=${2:-}

if [ ! -f "$file" ]; then
	echo "ilk archive: no such file: $file" >&2
	exit 1
fi

case "$file" in
"$archive_dir"/*)
	echo "ilk archive: $file is already archived" >&2
	exit 1
	;;
esac

mkdir -p "$archive_dir"
dest="$archive_dir/$(basename "$file")"

if [ -e "$dest" ]; then
	echo "ilk archive: $dest already exists — rename one of them first" >&2
	exit 1
fi

today=$(date +%Y-%m-%d)

# Mark the document as superseded inside its frontmatter, so it is self-describing
# wherever somebody finds it later.
tmp="$file.ilk-archive.$$"
awk -v today="$today" -v replacement="$replacement" '
	NR == 1 && $0 == "---" { print; infm = 1; next }
	infm && $0 == "---" {
		print "status: superseded"
		print "archived: " today
		if (replacement != "") print "superseded_by: " replacement
		print
		infm = 0
		next
	}
	infm && /^status:/ { next }
	infm && /^archived:/ { next }
	infm && /^superseded_by:/ { next }
	{ print }
' "$file" >"$tmp"

mv "$tmp" "$file"

if git ls-files --error-unmatch "$file" >/dev/null 2>&1; then
	git mv "$file" "$dest"
else
	mv "$file" "$dest"
fi

echo "Archived: $dest"
if [ -n "$replacement" ]; then
	echo "  superseded_by: $replacement"
else
	echo
	echo "No replacement recorded. If something replaced this, re-run with its id so a"
	echo "reader who finds the archived document knows where to go next."
fi
echo
echo "Now check that nothing live still links to it:"
echo "  ilk check --only archive.no-live-links"
