#!/bin/sh
# The scanner of last resort.
#
# `secrets.command` names whatever this project already runs — gitleaks,
# trufflehog, a hosted service. This is the floor underneath it: a short list of
# credential shapes that are almost never anything else, so the layer catches
# something on the day it lands and before anybody has configured anything. It
# is not a maintained scanner and does not pretend to be one. If this repository
# holds anything worth stealing, declare a real one.
#
# It never prints what it matched. A scanner that echoes a credential into a CI
# log has not found the leak, it has widened it.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

cd "${ILK_REPO_ROOT:-.}"
allowlist=${ILK_VAR_ALLOWLIST:-.ilk/secrets-allow}

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT INT TERM

# What to read. `git ls-files` is the index, so a file staged for the commit
# being made is in the list and a build artifact is not — which is what makes
# this usable from a pre-commit hook. Outside a work tree (a sandbox, an
# unpacked tarball) fall back to the tree itself.
#
# Paths containing a newline are not handled and will simply not be scanned.
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	git ls-files -c >"$work/files"
else
	find . -name .git -type d -prune -o -type f -print | sed 's|^\./||' >"$work/files"
fi

# The allowlist is substrings, not globs: a path containing one is skipped.
# Every entry is a claim that a match there is not a credential, which is why
# they belong in a file somebody reviews rather than in this script.
if [ -f "$allowlist" ]; then
	grep -v -e '^[[:space:]]*#' -e '^[[:space:]]*$' "$allowlist" >"$work/allow" || :
	if [ -s "$work/allow" ]; then
		grep -F -v -f "$work/allow" "$work/files" >"$work/kept" || :
		mv "$work/kept" "$work/files"
	fi
fi

# Deliberately few and deliberately specific. A pattern that fires on ordinary
# code teaches people to pass --no-verify by reflex, which costs every gate in
# the repository and not only this one.
#
# Fields are label<TAB>pattern; patterns are POSIX extended regular expressions.
tab=$(printf '\t')
cat >"$work/patterns" <<'PATTERNS'
an AWS access key id	(AKIA|ASIA|AIDA|AGPA)[0-9A-Z]{16}
a private key	-----BEGIN [A-Z ]*PRIVATE KEY-----
a GitHub token	gh[pousr]_[A-Za-z0-9]{36}
a Slack token	xox[abprs]-[0-9A-Za-z-]{12,}
a Google API key	AIza[0-9A-Za-z_-]{35}
a hardcoded AWS secret access key	(aws|AWS)_(secret|SECRET)_(access|ACCESS)_(key|KEY)[[:space:]]*[:=][[:space:]]*["']?[A-Za-z0-9/+]{40}
PATTERNS

# One combined pass per file decides whether a file is worth looking at twice;
# the second pass only runs on the few that matched, and only to name which
# shape it was. Alternation composes, so joining the patterns changes nothing
# about what they mean.
combined=$(cut -f2- "$work/patterns" | paste -s -d '|' -)

# Findings are collected rather than printed as they are found, so they can be
# printed last. A caller that trims long output keeps the tail, and the tail is
# where the paths have to be — the guidance is repeated in the check's `fix:`,
# but nothing else knows which file it was.
: >"$work/findings"

while IFS= read -r file; do
	[ -f "$file" ] || continue
	grep -q -E -I -e "$combined" -- "$file" 2>/dev/null || continue

	while IFS="$tab" read -r label pattern; do
		[ -n "$pattern" ] || continue
		grep -n -E -I -e "$pattern" -- "$file" 2>/dev/null | cut -d: -f1 >"$work/lines"
		while IFS= read -r line; do
			[ -n "$line" ] || continue
			printf '%s:%s: looks like %s\n' "$file" "$line" "$label" >>"$work/findings"
		done <"$work/lines"
	done <"$work/patterns"
done <"$work/files"

if [ ! -s "$work/findings" ]; then
	exit 0
fi

cat <<EOF
Rotate it first.

A credential is compromised the moment it is pushed. Deleting it in a new commit
leaves it in the history, in every clone and in every fork, and treating that
deletion as the fix is the mistake this layer exists to prevent.

  1. Rotate the credential now. Everything after that is cleanup.
  2. Then purge it from history (git filter-repo, BFG). Forks, pull requests and
     caches may keep a copy regardless.
  3. Then rotate again if the purge was not immediate.

Never paste the value into an issue, a commit message or a pull request. Read the
handle-a-leaked-credential skill before doing anything else. If one of these is
not a credential — a fixture, a documented example — add its path to
$allowlist and say in the commit message why it is not one.

EOF
cat "$work/findings"
exit 1
