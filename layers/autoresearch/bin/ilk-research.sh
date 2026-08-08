#!/bin/sh
# Read the research directory's frontmatter and answer three questions about it.
#
#   ilk-research.sh sources        every finding cites something openable
#   ilk-research.sh expiry         no finding is past the date it declared
#   ilk-research.sh due [days]     what expires soon, before it fails
#
# The date questions are the reason this layer exists. Staleness elsewhere in the
# record is measured by coupling — a document goes stale when the code it describes
# changes — and that signal is simply absent here. A vendor halving a rate limit
# touches no file in this repository, so nothing local can ever notice it. A date the
# author declared is the only handle there is.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

mode=${1:-}
dir="${ILK_VAR_GROUP:-docs}/${ILK_VAR_RESEARCH_DIR:-research}"
warn_days=${2:-${ILK_VAR_WARN_BEFORE_DAYS:-14}}

case "$mode" in
sources | expiry | due) ;;
*)
	echo "usage: ilk-research.sh <sources|expiry|due [days]>" >&2
	exit 2
	;;
esac

case "$warn_days" in
*[!0-9]* | "")
	echo "ilk autoresearch due: days must be a whole number, got '$warn_days'" >&2
	exit 2
	;;
esac

# day_number converts YYYY-MM-DD to a count of days, so two dates can be compared and
# subtracted without `date -d`, which GNU and BSD disagree about.
day_number() {
	echo "$1" | awk -F- '{
		y = $1 + 0; m = $2 + 0; d = $3 + 0
		if (m <= 2) { y -= 1; m += 12 }
		era = int(y / 400); yoe = y - era * 400
		doy = int((153 * (m - 3) + 2) / 5) + d - 1
		doe = yoe * 365 + int(yoe / 4) - int(yoe / 100) + doy
		print era * 146097 + doe - 719468
	}'
}

# read_meta prints `expires=<value>` and `urls=<count>` for one document.
#
# It reads only the frontmatter block. A URL in the prose is evidence of nothing,
# because nobody can tell from it whether the page was read or merely mentioned.
read_meta() {
	awk '
	NR == 1 { if ($0 ~ /^---[ \t]*$/) { fm = 1; next } else { exit } }
	!fm { next }
	$0 ~ /^---[ \t]*$/ { fm = 0; next }
	/^[A-Za-z_][A-Za-z0-9_-]*:/ {
		key = $0; sub(/:.*/, "", key)
		rest = $0; sub(/^[^:]*:[ \t]*/, "", rest)
		if (key == "expires") { expires = rest }
		if (key == "sources") { urls += gsub(/https?:\/\//, "&", rest) }
		next
	}
	key == "sources" { urls += gsub(/https?:\/\//, "&", $0) }
	END {
		sub(/[ \t]+$/, "", expires)
		print "expires=" expires
		print "urls=" (urls + 0)
	}
	' "$1"
}

meta_field() { read_meta "$1" | sed -n "s/^$2=//p" | tr -d "\"'"; }

today=$(day_number "$(date +%Y-%m-%d)")

# examine prints one line per thing worth saying about a document, and nothing at
# all when there is nothing to say. Whether the run passes is then just whether it
# printed anything, which keeps the verdict and the message from ever disagreeing.
examine() {
	file=$1

	if [ "$mode" = sources ]; then
		urls=$(meta_field "$file" urls)
		[ "${urls:-0}" -eq 0 ] || return 0
		echo "$file: nothing in \`sources:\` that anybody else can open"
		return 0
	fi

	expires=$(meta_field "$file" expires)
	case "$expires" in
	[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]) ;;
	"")
		# Silent for `due`: a finding with no date is not expiring soon, it is
		# unmeasurable, and research.fresh is where that is said.
		[ "$mode" = expiry ] || return 0
		echo "$file: no \`expires:\` date, so nothing can ever mark it out of date"
		return 0
		;;
	*)
		[ "$mode" = expiry ] || return 0
		echo "$file: \`expires: $expires\` is not a YYYY-MM-DD date"
		return 0
		;;
	esac

	left=$(($(day_number "$expires") - today))
	if [ "$mode" = expiry ]; then
		[ "$left" -lt 0 ] || return 0
		echo "$file: expired $((0 - left)) day(s) ago ($expires), and is still being read as current"
		return 0
	fi

	[ "$left" -le "$warn_days" ] || return 0
	if [ "$left" -lt 0 ]; then
		printf '  %-52s %s\n' "$file" "expired $((0 - left)) day(s) ago"
	else
		printf '  %-52s %s\n' "$file" "$left day(s) left"
	fi
}

# Recursive, because the built-in checks over this directory are, and a finding
# filed in a subdirectory that half the checks cannot see is the worst of both.
# A directory nobody has written in yet is not a failure.
scan() {
	[ -d "$dir" ] || return 0
	find "$dir" -type f -name '*.md' | LC_ALL=C sort | while IFS= read -r file; do
		case "${file##*/}" in README.md) continue ;; esac
		examine "$file"
	done
}

report=$(scan)

if [ "$mode" = due ]; then
	if [ -n "$report" ]; then
		printf '%s\n' "$report"
	else
		echo "  nothing in $dir/ expires in the next $warn_days day(s)"
	fi
	exit 0
fi

[ -n "$report" ] || exit 0
printf '%s\n' "$report"
exit 1
