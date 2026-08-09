#!/bin/sh
# Read every deprecation notice in the record and answer one question about it.
#
#   overdue  fail on a notice whose date has passed while its code is still here
#   done     fail on a notice whose paths have all gone, so it can be retired
#   list     print where every notice stands, and never fail
#
# The question both checks ask is the same one `record.stale` asks, pointed at
# removal instead of review: the document declares what it is about, and git says
# whether that subject is still there.
#
# Usage: ilk-deprecation-scan.sh <mode> [docs-dir] [prefix]
set -eu

mode=${1:-list}
docs_dir=${2:-docs/reference}
prefix=${3:-DEPRECATED-}

today=$(date +%Y-%m-%d)
today_n=$(echo "$today" | tr -d '-')

# Paths are matched against what git tracks, the same way `covers:` is matched
# everywhere else in ilk. Outside a git repository — a sandbox, a fresh
# directory — there is nothing to ask, so the working tree answers instead.
use_git=0
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	if [ -n "$(git ls-files | head -n 1)" ]; then
		use_git=1
	fi
fi

# matches_any reports whether one `covers:` pattern still names something real.
matches_any() {
	if [ "$use_git" = 1 ]; then
		[ -n "$(git ls-files -- ":(glob)$1" 2>/dev/null | head -n 1)" ]
	else
		[ -n "$(find . -path ./.git -prune -o -path "./$1" -print 2>/dev/null | head -n 1)" ]
	fi
}

# read_meta prints the frontmatter this layer cares about, one tab-separated
# record per line: `remove_after<TAB>date` and one `cover<TAB>pattern` per path.
# Both the block and the inline form of a YAML list are read.
read_meta() {
	awk '
		function clean(s,   q, n) {
			q = "\047"
			gsub(/^[[:blank:]]+|[[:blank:]]+$/, "", s)
			if (substr(s, 1, 1) == "\"" || substr(s, 1, 1) == q) { s = substr(s, 2) }
			n = length(s)
			if (n > 0 && (substr(s, n, 1) == "\"" || substr(s, n, 1) == q)) { s = substr(s, 1, n - 1) }
			return s
		}
		NR == 1 { if ($0 !~ /^---[[:blank:]]*$/) { exit } ; next }
		/^---[[:blank:]]*$/ { exit }
		/^covers:/ {
			rest = clean(substr($0, 8))
			if (rest ~ /^\[/) {
				sub(/^\[/, "", rest)
				sub(/\][[:blank:]]*$/, "", rest)
				n = split(rest, item, ",")
				for (i = 1; i <= n; i++) {
					if (clean(item[i]) != "") { print "cover\t" clean(item[i]) }
				}
			}
			incovers = 1
			next
		}
		incovers && /^[[:blank:]]+-[[:blank:]]*/ {
			line = $0
			sub(/^[[:blank:]]+-[[:blank:]]*/, "", line)
			if (clean(line) != "") { print "cover\t" clean(line) }
			next
		}
		/^[A-Za-z_]/ { incovers = 0 }
		/^remove_after:/ { print "remove_after\t" clean(substr($0, 14)) }
	' "$1"
}

# cover_stats reduces a notice's patterns, read from stdin, to
# "<declared> <still-matching> <first-matching-pattern>".
cover_stats() {
	declared=0
	matching=0
	example=
	while IFS= read -r pattern; do
		[ -n "$pattern" ] || continue
		declared=$((declared + 1))
		if matches_any "$pattern"; then
			matching=$((matching + 1))
			[ -n "$example" ] || example=$pattern
		fi
	done
	printf '%s %s %s\n' "$declared" "$matching" "$example"
}

[ -d "$docs_dir" ] || exit 0
notices=$(find "$docs_dir" -type f -name "$prefix*.md" | LC_ALL=C sort)

status=0
while IFS= read -r file; do
	[ -n "$file" ] || continue

	meta=$(read_meta "$file")
	remove_after=$(printf '%s\n' "$meta" | awk -F'\t' '$1 == "remove_after" { print $2; exit }')
	covers=$(printf '%s\n' "$meta" | awk -F'\t' '$1 == "cover" { print $2 }')

	stats=$(printf '%s\n' "$covers" | cover_stats)
	declared=${stats%% *}
	rest=${stats#* }
	matching=${rest%% *}
	example=${rest#* }

	case "$mode" in
	overdue)
		# A missing date belongs to deprecation.frontmatter; saying so twice
		# would send a reader to two places for one repair.
		[ -n "$remove_after" ] || continue

		# A date this cannot read and a `covers:` with nothing in it are both
		# reported here, because both leave this check unable to answer its own
		# question — and a check that quietly passes when it cannot evaluate is
		# indistinguishable from one that works.
		case "$remove_after" in
		[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]) ;;
		*)
			printf '%s: remove_after %s is not a date this can compare — write it as YYYY-MM-DD\n' "$file" "$remove_after"
			status=1
			continue
			;;
		esac
		if [ "$declared" -eq 0 ]; then
			printf '%s: covers is empty, so nothing can ever hold this deprecation to its date\n' "$file"
			status=1
			continue
		fi

		if [ "$today_n" -gt "$(echo "$remove_after" | tr -d '-')" ] && [ "$matching" -gt 0 ]; then
			printf '%s: remove_after %s has passed and %s is still here\n' "$file" "$remove_after" "$example"
			status=1
		fi
		;;
	'done')
		# Nothing declared means nothing to conclude; overdue reports that.
		[ "$declared" -gt 0 ] || continue
		if [ "$matching" -eq 0 ]; then
			printf '%s: nothing it covers is left — the removal is finished\n' "$file"
			status=1
		fi
		;;
	list)
		state=due
		if [ "$declared" -gt 0 ] && [ "$matching" -eq 0 ]; then
			state='done'
		else
			case "$remove_after" in
			[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9])
				if [ "$today_n" -gt "$(echo "$remove_after" | tr -d '-')" ]; then
					state=OVERDUE
				fi
				;;
			*) state=unchecked ;;
			esac
		fi
		printf '%-9s %-12s %s\n' "$state" "${remove_after:-no-date}" "$file"
		;;
	*)
		echo "usage: ilk-deprecation-scan.sh <overdue|done|list> [docs-dir] [prefix]" >&2
		exit 2
		;;
	esac
done <<EOF
$notices
EOF

exit "$status"
