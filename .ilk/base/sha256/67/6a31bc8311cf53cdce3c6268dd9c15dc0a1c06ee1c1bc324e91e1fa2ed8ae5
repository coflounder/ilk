#!/bin/sh
# Open a drafted proposal as a pull request on the layer's own repository.
#
# ilk decides what a proposal is, when it is ready, and what must never be in one.
# This script knows only what a pull request is — the same split as a mirror, and
# the reason ilk itself never talks to GitHub.
#
# It reads the finished proposal on stdin and takes everything else from the
# environment:
#
#   ILK_PROPOSAL_TITLE   the pull request's title
#   ILK_PROPOSAL_BRANCH  the branch to push
#   ILK_PROPOSAL_SLUG    the layer id, safe for a filename
#   ILK_PROPOSAL_FROM    the repository the proposal came from
#   ILK_CONTRIB_REPO     owner/name of the layer's repository
#   ILK_CONTRIB_PATH     where the layer lives inside it
#   ILK_CONTRIB_BASE     the branch to target
#
# It prints the pull request's URL and nothing else.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

title=${ILK_PROPOSAL_TITLE:-}
branch=${ILK_PROPOSAL_BRANCH:-}
slug=${ILK_PROPOSAL_SLUG:-proposal}
from=${ILK_PROPOSAL_FROM:-unknown}
repo=${ILK_CONTRIB_REPO:-}
layer_path=${ILK_CONTRIB_PATH:-}
base=${ILK_CONTRIB_BASE:-main}

die() {
	echo "ilk-propose: $1" >&2
	exit 1
}

command -v gh >/dev/null 2>&1 ||
	die "the GitHub CLI (gh) is not installed — see https://cli.github.com.
  This is the default way to open a proposal; a layer can name another by setting
  \`contribution.submit\` in its manifest."
gh auth status >/dev/null 2>&1 ||
	die "gh is not authenticated. Run \`gh auth login\`."
[ -n "$repo" ] || die "no upstream repository given"
[ -n "$title" ] || die "no title given"
[ -n "$branch" ] || die "no branch given"

# Hold the proposal before doing anything that can fail: stdin is readable once.
proposal=$(cat)
[ -n "$proposal" ] || die "the proposal is empty"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# Fork first. Most contributors cannot push to the repository they are proposing
# to, and `gh repo fork` is a no-op when the fork already exists — which is the
# normal case by the second proposal.
gh repo fork "$repo" --clone=false --remote=false >/dev/null 2>&1 ||
	die "could not fork $repo. Check that it exists and that your token can create forks."

me=$(gh api user --jq '.login') || die "could not read your GitHub login"
fork="$me/$(basename "$repo")"

gh repo clone "$fork" "$work/repo" -- --depth=1 --branch "$base" >/dev/null 2>&1 ||
	gh repo clone "$fork" "$work/repo" -- --depth=1 >/dev/null 2>&1 ||
	die "could not clone $fork"

cd "$work/repo"

# Take the upstream base, so a fork that has gone stale does not silently propose
# against a months-old tree. The URL comes from gh rather than being assembled from
# the repository name, so this works against an enterprise host too.
upstream_url=$(gh repo view "$repo" --json url --jq '.url') ||
	die "could not resolve $repo"
git remote add upstream "$upstream_url" >/dev/null 2>&1 || true
git fetch --depth=1 upstream "$base" >/dev/null 2>&1 ||
	die "could not fetch $base from $repo"
git checkout -q -B "$branch" FETCH_HEAD

# The proposal is committed as a file rather than left in the pull request body
# alone. That is what lets the maintainer's own checks run on it in CI: a proposal
# nobody can validate is a proposal somebody has to read carefully by hand.
mkdir -p proposals
target="proposals/${slug}--from-$(printf '%s' "$from" | tr -c 'A-Za-z0-9._-' '-').md"
printf '%s' "$proposal" > "$target"

git add "$target"
git -c user.email="$(gh api user --jq '.email // "noreply@github.com"')" \
	-c user.name="$me" \
	commit -q -m "$title" -m "Proposed by ilk contribute from $from, about ${layer_path:-$repo}."

# Fetch the branch's own tracking ref before forcing, so --force-with-lease has
# something to compare against. Without it a shallow clone knows nothing about the
# branch and the lease refuses — which is exactly the second time somebody sends a
# corrected proposal, the case that must not be the one that breaks.
git fetch --depth=1 origin "+refs/heads/$branch:refs/remotes/origin/$branch" >/dev/null 2>&1 || true
git push -q --force-with-lease origin "$branch" ||
	die "could not push $branch to $fork. If somebody else changed that branch, delete it in your fork and re-run."

url=$(gh pr create --repo "$repo" --base "$base" --head "$me:$branch" \
	--title "$title" --body "$proposal") ||
	die "pushed $branch to $fork but could not open the pull request.
  Open it by hand against $repo, or re-run once the reason is fixed — the branch is already there."

echo "$url"
