#!/bin/sh
# Finished work cites evidence somebody else can open.
#
# `ilk pr-prep draft` copies the `## Evidence` section into the pull request
# description word for word. "Tested locally, works fine" survives that trip
# intact and gives the reviewer nothing to do, which is how a description ends up
# being taken on trust — so this refuses it. A bullet has to carry something
# openable: a link, a URL, a command in backticks, or a #reference.
#
# It says nothing about documents that are still being worked on. An unproven
# claim is only a problem once somebody is asked to believe it.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

dir=${ILK_PLANS_DIR:-docs/plans}
want=${ILK_VAR_SHIP_STATUS:-done}

[ -d "$dir" ] || exit 0

failed=0
for file in "$dir"/*.md; do
	[ -e "$file" ] || continue
	case "${file##*/}" in
	README.md) continue ;;
	esac

	verdict=$(awk -v want="$want" '
		BEGIN { q = sprintf("%c", 39) }
		NR == 1 && $0 != "---" { stop = 1; exit }
		NR == 1 { fm = 1; next }
		fm && $0 == "---" { fm = 0; next }
		fm {
			if ($0 !~ /^status:/) next
			st = $0
			sub(/^status:[ \t]*/, "", st)
			gsub(/"/, "", st)
			gsub(q, "", st)
			sub(/[ \t]+$/, "", st)
			st = tolower(st)
			next
		}
		/^#+[ \t]+/ {
			h = tolower($0)
			sub(/^#+[ \t]+/, "", h)
			sub(/[ \t]+$/, "", h)
			insec = (h == "evidence")
			if (insec) seen = 1
			next
		}
		insec {
			if ($0 ~ /^[ \t]*([-*+]|[0-9]+\.)[ \t]+[^ \t]/) items++
			if ($0 ~ /https?:\/\// || $0 ~ /\]\(/ || $0 ~ /`[^`]+`/ || $0 ~ /#[0-9]+/) openable++
		}
		END {
			if (stop) exit
			if (st != tolower(want)) exit
			if (!seen) { print "no-section"; exit }
			if (!items) { print "no-items"; exit }
			if (!openable) { print "unopenable"; exit }
		}
	' "$file")

	case "$verdict" in
	no-section)
		echo "$file: is $want and has no \`## Evidence\` section, so a description drafted from it would cite nothing"
		;;
	no-items)
		echo "$file: has an empty \`## Evidence\` section"
		;;
	unopenable)
		echo "$file: cites nothing anybody can open — no link, no command in backticks, no reference"
		;;
	*)
		continue
		;;
	esac
	failed=1
done

[ "$failed" -eq 0 ] || exit 1
