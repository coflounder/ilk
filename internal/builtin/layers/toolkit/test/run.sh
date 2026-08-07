#!/bin/sh
# Exercise ilk-propose.sh against a fake gh and real local git repositories.
#
# What this proves: the script's contract with ilk — that it reads the proposal from
# stdin and the rest from ILK_* variables, commits the proposal where the maintainer
# layer's checks will find it, targets the right base, and prints the pull request's
# URL and nothing else. The git half is genuinely run; only the network is faked.
#
# What it cannot prove is that the gh invocations are the ones the real gh wants.
# The fake is strict about unrecognised calls so a changed invocation fails here
# rather than silently.
#
#   sh internal/builtin/layers/toolkit/test/run.sh
set -eu

here=$(cd "$(dirname "$0")" && pwd)
script=$here/../bin/ilk-propose.sh
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

PATH=$here:$PATH
export PATH

# Upstream: a repository holding a layer. Fork: what the contributor pushes to.
git init -q --bare "$work/upstream.git"
seed=$work/seed
git init -q "$seed"
git -C "$seed" config user.email t@example.com
git -C "$seed" config user.name Test
mkdir -p "$seed/layers/demo"
echo "id: demo/layer" > "$seed/layers/demo/layer.yaml"
git -C "$seed" add -A
git -C "$seed" commit -qm "the layer"
git -C "$seed" branch -M main
git -C "$seed" push -q "$work/upstream.git" main

git clone -q --bare "$work/upstream.git" "$work/fork.git"

GH_FAKE_LOG=$work/gh.log
GH_FAKE_UPSTREAM=$work/upstream.git
GH_FAKE_FORK=$work/fork.git
GH_FAKE_PR_BODY=$work/pr-body
GH_FAKE_PR_TITLE=$work/pr-title
GH_FAKE_PR_HEAD=$work/pr-head
GH_FAKE_PR_BASE=$work/pr-base
export GH_FAKE_LOG GH_FAKE_UPSTREAM GH_FAKE_FORK
export GH_FAKE_PR_BODY GH_FAKE_PR_TITLE GH_FAKE_PR_HEAD GH_FAKE_PR_BASE
: > "$GH_FAKE_LOG"

ILK_PROPOSAL_TITLE="demo/layer: what acme-api learned"
ILK_PROPOSAL_BRANCH="ilk-proposal/demo-layer"
ILK_PROPOSAL_SLUG="demo-layer"
ILK_PROPOSAL_FROM="acme-api"
ILK_CONTRIB_REPO="acme/layers"
ILK_CONTRIB_PATH="layers/demo"
ILK_CONTRIB_BASE="main"
export ILK_PROPOSAL_TITLE ILK_PROPOSAL_BRANCH ILK_PROPOSAL_SLUG ILK_PROPOSAL_FROM
export ILK_CONTRIB_REPO ILK_CONTRIB_PATH ILK_CONTRIB_BASE

proposal='---
layer: demo/layer
from: acme-api
status: open
---

# demo/layer: what acme-api learned

## What this repository needed

The default was wrong for a monorepo.
'

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

echo "ilk-propose"

url=$(printf '%s' "$proposal" | sh "$script")
check "prints the pull request url and nothing else" \
	"$url" "https://github.com/acme/layers/pull/7"

check "opens against the configured base" "$(cat "$work/pr-base")" "main"
check "opens from the contributor's fork" "$(cat "$work/pr-head")" "contributor:ilk-proposal/demo-layer"
check "carries the title ilk chose" "$(cat "$work/pr-title")" "$ILK_PROPOSAL_TITLE"

# The proposal is a file in the branch, not only a pull request body. That is what
# lets the maintainer layer's checks run on it in CI.
committed=$(git -C "$work/fork.git" show "ilk-proposal/demo-layer:proposals/demo-layer--from-acme-api.md")
check "commits the proposal where the maintainer layer looks for it" \
	"$(printf '%s' "$committed" | sed -n '2p')" "layer: demo/layer"

# The branch is built on upstream's base, not on whatever the fork had.
parent=$(git -C "$work/fork.git" rev-parse "ilk-proposal/demo-layer^")
upstream_head=$(git -C "$work/upstream.git" rev-parse main)
check "branches from upstream's base, not from a stale fork" "$parent" "$upstream_head"

# The pull request body is the proposal, so a reviewer reads the case without
# clicking into the diff.
check "the body is the proposal itself" \
	"$(head -1 "$work/pr-body")" "---"

# Re-running must not fail on the branch already existing: a contributor who fixes
# a sentence and re-sends is the normal case, not an error.
: > "$GH_FAKE_LOG"
url=$(printf '%s' "$proposal" | sh "$script")
check "re-sending an updated proposal works" \
	"$url" "https://github.com/acme/layers/pull/7"

# An empty proposal is a bug upstream in ilk, and silently opening an empty pull
# request would be the worst possible response to it.
if out=$(printf '' | sh "$script" 2>&1); then
	echo "  ✗ an empty proposal is refused"
	failures=$((failures + 1))
else
	case $out in
	*empty*) echo "  ✓ an empty proposal is refused" ;;
	*)
		echo "  ✗ an empty proposal is refused"
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
