#!/bin/sh
# Show the specs that are ready to work on, grouped by milestone.
#
#   ilk blueprint next [--all]
#
# Derived from the plan documents every time it runs, so it cannot go stale the way a
# status report written last week goes stale.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

dir=${ILK_VAR_PLANS_DIR:-plans}
show_all=0
for arg in "$@"; do
	case "$arg" in
	--all) show_all=1 ;;
	-h | --help)
		echo "usage: ilk blueprint next [--all]" >&2
		echo "  --all   include specs that are done or dropped" >&2
		exit 2
		;;
	*)
		echo "ilk blueprint next: unknown argument $arg" >&2
		exit 2
		;;
	esac
done

if [ ! -d "$dir" ]; then
	echo "ilk blueprint next: no $dir/ directory — run \`ilk apply\` first" >&2
	exit 1
fi

# One record per spec: milestone, status, criteria count, title, path.
scan() {
	for file in "$dir"/SPEC-*.md; do
		[ -e "$file" ] || continue
		awk -v path="$file" '
			NR == 1 && $0 != "---" { exit }
			NR == 1 { infm = 1; next }
			infm && $0 == "---" { infm = 0; next }
			infm && /^milestone:/ { m = $0; sub(/^milestone:[[:space:]]*/, "", m); gsub(/["'"'"']/, "", m) }
			infm && /^status:/    { s = $0; sub(/^status:[[:space:]]*/, "", s); gsub(/["'"'"']/, "", s) }
			infm && /^title:/     { t = $0; sub(/^title:[[:space:]]*/, "", t); gsub(/["'"'"']/, "", t) }
			!infm && /^#{1,6}[[:space:]]+[Aa]cceptance criteria[[:space:]]*$/ { insec = 1; next }
			!infm && insec && /^#/ { insec = 0 }
			!infm && insec && /^[[:space:]]*([-*+]|[0-9]+\.)[[:space:]]+[^[:space:]]/ { n++ }
			END {
				if (m == "") m = "(none)"
				if (s == "") s = "(none)"
				printf "%s\t%s\t%d\t%s\t%s\n", m, s, n, t, path
			}
		' "$file"
	done
}

rows=$(scan | sort -t"$(printf '\t')" -k1,1)

if [ -z "$rows" ]; then
	echo "No specs in $dir/ yet. Create one with:"
	echo "  ilk record new $dir \"What you are about to build\""
	exit 0
fi

current=""
shown=0
printf '%s\n' "$rows" | while IFS="$(printf '\t')" read -r milestone status criteria title path; do
	case "$status" in
	done | dropped)
		[ "$show_all" -eq 1 ] || continue
		;;
	esac

	if [ "$milestone" != "$current" ]; then
		[ -z "$current" ] || echo
		echo "$milestone"
		current=$milestone
	fi

	note=""
	if [ "$criteria" -eq 0 ]; then
		note="  ← no acceptance criteria; specify it before starting"
	fi
	printf '  %-10s %-3s %s%s\n' "$status" "$criteria" "$title" "$note"
	printf '  %-10s %-3s %s\n' "" "" "$path"
	shown=$((shown + 1))
done

echo
echo "Columns: status, acceptance criteria count, title."
if [ "$show_all" -eq 0 ]; then
	echo "Done and dropped specs are hidden; pass --all to see them."
fi
