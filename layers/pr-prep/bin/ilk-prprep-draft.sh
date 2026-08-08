#!/bin/sh
# Assemble a pull request description from a plan document.
#
#   ilk pr-prep draft <document> [--title]
#
# Everything in the description is read out of the document on the spot — the
# title, the outcome, the acceptance criteria, the evidence. Nothing is
# summarised from the diff, because a summary written from the diff is where the
# acceptance criteria go missing.
#
# The body goes to stdout and every remark goes to stderr, so
#
#   ilk pr-prep draft SPEC-webhooks > body.md
#
# leaves a file `gh pr create --body-file body.md` will take.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

dir=${ILK_PLANS_DIR:-docs/plans}
title_only=0
target=""

usage() {
	echo "usage: ilk pr-prep draft <document> [--title]" >&2
	echo "  --title   print the pull request title instead of the body" >&2
}

# One frontmatter field, unquoted. Stops at the closing delimiter so a line in
# the body that looks like `status: whatever` cannot answer for the header.
field() {
	awk -v key="$1" '
		BEGIN { q = sprintf("%c", 39) }
		NR == 1 && $0 != "---" { exit }
		NR == 1 { next }
		$0 == "---" { exit }
		{
			k = $0
			sub(/:.*$/, "", k)
			sub(/[ \t]+$/, "", k)
			if (k != key) next
			v = $0
			sub(/^[^:]*:[ \t]*/, "", v)
			gsub(/"/, "", v)
			gsub(q, "", v)
			sub(/[ \t]+$/, "", v)
			print v
			exit
		}
	' "$2"
}

# Strip the blank lines off both ends, so sections compose without gaps.
trim() {
	awk '
		{ line[NR] = $0 }
		END {
			for (i = 1; i <= NR; i++)
				if (line[i] ~ /[^ \t]/) {
					if (!first) first = i
					last = i
				}
			if (!first) exit
			for (i = first; i <= last; i++) print line[i]
		}
	'
}

# The body under a named heading, at any level, matched case-insensitively.
section() {
	awk -v want="$1" '
		NR == 1 && $0 == "---" { fm = 1; next }
		fm && $0 == "---" { fm = 0; next }
		fm { next }
		/^#+[ \t]+/ {
			h = tolower($0)
			sub(/^#+[ \t]+/, "", h)
			sub(/[ \t]+$/, "", h)
			insec = (h == want)
			next
		}
		insec { print }
	' "$2" | trim
}

# The opening paragraph of the body — the fallback when a document names no
# section this command knows, which is common in a repository that writes plans
# its own way.
first_para() {
	awk '
		NR == 1 && $0 == "---" { fm = 1; next }
		fm && $0 == "---" { fm = 0; next }
		fm { next }
		/^<!--/ { comment = 1 }
		comment { if ($0 ~ /-->/) comment = 0; next }
		/^#/ { if (started) exit; next }
		/^[ \t]*$/ { if (started) exit; next }
		{ print; started = 1 }
	' "$1"
}

# What could be drafted from, with the status that decides whether it should be.
candidates() {
	found=0
	for file in "$dir"/*.md; do
		[ -e "$file" ] || continue
		case "${file##*/}" in
		README.md) continue ;;
		esac
		printf '  %-10s %s\n' "$(field status "$file")" "$file"
		found=1
	done
	[ "$found" -eq 1 ] || echo "  (none — $dir/ has no plan documents yet)"
}

for arg in "$@"; do
	case "$arg" in
	--title) title_only=1 ;;
	-h | --help)
		usage
		exit 2
		;;
	-*)
		echo "ilk pr-prep draft: unknown argument $arg" >&2
		usage
		exit 2
		;;
	*)
		if [ -n "$target" ]; then
			echo "ilk pr-prep draft: one document at a time (got $target and $arg)" >&2
			exit 2
		fi
		target=$arg
		;;
	esac
done

if [ ! -d "$dir" ]; then
	echo "ilk pr-prep draft: no $dir/ directory — run \`ilk apply\` first" >&2
	exit 1
fi

if [ -z "$target" ]; then
	echo "ilk pr-prep draft: name the document this change comes from." >&2
	echo >&2
	candidates >&2
	exit 2
fi

doc=""
for candidate in "$target" "$dir/$target" "$dir/$target.md"; do
	if [ -f "$candidate" ]; then
		doc=$candidate
		break
	fi
done
if [ -z "$doc" ]; then
	echo "ilk pr-prep draft: no document called $target in $dir/" >&2
	echo >&2
	candidates >&2
	exit 1
fi

title=$(field title "$doc")
[ -n "$title" ] || title=$(basename "$doc" .md)

if [ "$title_only" -eq 1 ]; then
	echo "$title"
	exit 0
fi

status=$(field status "$doc")
summary=$(section outcome "$doc")
[ -n "$summary" ] || summary=$(section summary "$doc")
[ -n "$summary" ] || summary=$(section context "$doc")
[ -n "$summary" ] || summary=$(first_para "$doc")
criteria=$(section "acceptance criteria" "$doc")
evidence=$(section evidence "$doc")

echo "## What this changes"
echo
if [ -n "$summary" ]; then
	echo "$summary"
else
	echo "_The document states no outcome._"
fi
echo
echo "## Acceptance criteria"
echo
if [ -n "$criteria" ]; then
	echo "$criteria"
else
	echo "_The document states no acceptance criteria, so nothing here says what would settle this change._"
fi
echo
echo "## Evidence"
echo
if [ -n "$evidence" ]; then
	echo "$evidence"
else
	echo "_No evidence recorded._"
fi
echo
echo "---"
echo
echo "Derived from \`$doc\` (status: ${status:-unknown}) by \`ilk pr-prep draft\`."

# Remarks go to stderr so they never reach the pull request. Each one is a gap in
# the record that the description has just made visible; the description is not
# the place to fix it.
if [ -z "$criteria" ]; then
	echo "ilk pr-prep draft: $doc states no acceptance criteria — a reviewer has nothing to accept against." >&2
fi
if printf '%s\n' "$criteria" | grep -qF -- '- [ ]'; then
	echo "ilk pr-prep draft: $doc still has unticked acceptance criteria; the description says so verbatim." >&2
fi
if [ -z "$evidence" ]; then
	echo "ilk pr-prep draft: $doc records no evidence — add what you ran and what it said before opening the request." >&2
fi
