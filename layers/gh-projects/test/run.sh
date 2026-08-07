#!/bin/sh
# Exercise gh-projects.sh against a fake gh.
#
# What this proves: the script's contract with ilk — that it reads ILK_MIRROR_*,
# prints an id ilk can record, and refuses rather than inventing a status the board
# does not offer. What it cannot prove is that the `gh` invocations are the right
# ones; only a real project can do that. The fake is deliberately strict about
# unrecognised calls so a changed invocation fails here rather than silently.
#
#   sh layers/gh-projects/test/run.sh
set -eu

here=$(cd "$(dirname "$0")" && pwd)
script=$here/../bin/gh-projects.sh
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

PATH=$here:$PATH
export PATH
GH_FAKE_LOG=$work/gh.log
export GH_FAKE_LOG
: > "$GH_FAKE_LOG"

cat > "$work/items.json" <<'JSON'
{"items":[
 {"id":"ITEM_A","title":"Rate limiting","status":"Todo","content":{"url":"https://github.com/acme/api/issues/1"}}
]}
JSON
cat > "$work/fields.json" <<'JSON'
{"fields":[
 {"id":"FLD","name":"Status","options":[{"id":"OPT_TODO","name":"Todo"},{"id":"OPT_WIP","name":"In Progress"}]}
]}
JSON
GH_FAKE_ITEMS=$work/items.json
GH_FAKE_FIELDS=$work/fields.json
export GH_FAKE_ITEMS GH_FAKE_FIELDS

ILK_VAR_OWNER=acme
ILK_VAR_PROJECT_NUMBER=7
ILK_VAR_STATUS_FIELD=Status
ILK_VAR_REPO=
export ILK_VAR_OWNER ILK_VAR_PROJECT_NUMBER ILK_VAR_STATUS_FIELD ILK_VAR_REPO

failures=0
check() {
	if [ "$2" = "$3" ]; then
		echo "  ✓ $1"
	else
		echo "  ✗ $1"
		echo "      got:  $2"
		echo "      want: $3"
		failures=$((failures + 1))
	fi
}

echo "gh-projects provider"

# doctor names the project it reached, so a wrong number is visible before a sync.
out=$(sh "$script" doctor)
check "doctor reports the project it connected to" \
	"$out" 'gh-projects: connected to "API Roadmap" (acme/7)'

# list normalises to ilk's shape, with status lowercased for case-insensitive diffing.
out=$(sh "$script" list | jq -c '.')
check "list normalises the board to {id,title,status,url}" \
	"$out" '[{"id":"ITEM_A","title":"Rate limiting","status":"todo","url":"https://github.com/acme/api/issues/1"}]'

# create prints an id ilk can record. Without it the item is stranded on the board.
out=$(ILK_MIRROR_TITLE="Retry budgets" ILK_MIRROR_STATUS="in progress" \
	ILK_MIRROR_PATH="plans/SPEC-retries.md" sh "$script" create)
check "create prints the new item's id" "$out" '{"id":"ITEM_NEW"}'

# With a repo configured the item is a real issue, so commits can reference it.
out=$(ILK_VAR_REPO=acme/api ILK_MIRROR_TITLE="Retry budgets" \
	ILK_MIRROR_PATH="plans/SPEC-retries.md" sh "$script" create)
check "create makes a real issue when a repo is configured" \
	"$out" '{"id":"ITEM_FROM_ISSUE","url":"https://github.com/acme/api/issues/9"}'

# The status must reach the board on create, or the next run has to fix it.
: > "$GH_FAKE_LOG"
ILK_MIRROR_TITLE="Retry budgets" ILK_MIRROR_STATUS="in progress" sh "$script" create > /dev/null
if grep -q 'single-select-option-id OPT_WIP' "$GH_FAKE_LOG"; then
	echo "  ✓ create sets the status field"
else
	echo "  ✗ create sets the status field"
	failures=$((failures + 1))
fi

# Matching is case-insensitive: a document says "in progress", the board says
# "In Progress", and neither should have to know about the other's convention.
: > "$GH_FAKE_LOG"
ILK_MIRROR_ID=ITEM_A ILK_MIRROR_TITLE="Rate limiting" ILK_MIRROR_STATUS="IN PROGRESS" \
	sh "$script" update
if grep -q 'single-select-option-id OPT_WIP' "$GH_FAKE_LOG"; then
	echo "  ✓ update matches a status option regardless of case"
else
	echo "  ✗ update matches a status option regardless of case"
	failures=$((failures + 1))
fi

# A status the board does not offer is refused and the alternatives named. Inventing
# an option would put a value on somebody else's board that nobody chose.
if out=$(ILK_MIRROR_ID=ITEM_A ILK_MIRROR_STATUS="shipped" sh "$script" update 2>&1); then
	echo "  ✗ update refuses a status the board does not offer"
	failures=$((failures + 1))
else
	case $out in
	*'does not offer'*'Todo, In Progress'*) echo "  ✓ update refuses a status the board does not offer, and names the options" ;;
	*)
		echo "  ✗ update refuses a status the board does not offer, and names the options"
		echo "      got: $out"
		failures=$((failures + 1))
		;;
	esac
fi

# A create whose status cannot be set still prints the id. An item ilk cannot link to
# is an orphan the next run tries to create all over again.
out=$(ILK_MIRROR_TITLE="Circuit breakers" ILK_MIRROR_STATUS="shipped" sh "$script" create 2>/dev/null)
check "a status failure does not lose the id of a created item" "$out" '{"id":"ITEM_NEW"}'

# Missing configuration says which key to set and where.
if out=$(ILK_VAR_OWNER= sh "$script" list 2>&1); then
	echo "  ✗ an unconfigured owner is refused"
	failures=$((failures + 1))
else
	case $out in
	*'.ilk/config.yaml'*) echo "  ✓ an unconfigured owner names the key and the file to set it in" ;;
	*)
		echo "  ✗ an unconfigured owner names the key and the file to set it in"
		echo "      got: $out"
		failures=$((failures + 1))
		;;
	esac
fi

if [ "$failures" -gt 0 ]; then
	echo "$failures failing"
	exit 1
fi
echo "all passing"
