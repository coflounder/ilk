#!/bin/sh
# Print the preview URL for a branch.
#
#   ilk preview url [branch] [--upload]
#
# Without --upload nothing is built or sent anywhere: the URL is worked out from
# the branch, the Worker's name and the account's workers.dev subdomain, and is
# the URL the branch's preview alias has, or will have once CI uploads it.
#
# With --upload, and only inside CI, the branch is uploaded as a preview version
# with `wrangler versions upload --preview-alias <alias>`, and the URL is the one
# Wrangler reports. A version is not a deployment: production keeps serving what
# it served. Nothing in this layer ever runs `wrangler deploy`.
#
# Flags arrive from ilk as ILK_FLAG_UPLOAD; --json as ILK_OUTPUT=json.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
provider=${ILK_VAR_PROVIDER:-cloudflare}
upload=${ILK_FLAG_UPLOAD:-0}
json=0
[ "${ILK_OUTPUT:-text}" = json ] && json=1

jstr() {
	printf '"%s"' "$(printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g')"
}

# fail <code> <message> <fix>: the same shape ilk's own errors have.
fail() {
	if [ "$json" = 1 ]; then
		printf '{"error":{"code":%s,"message":%s,"exit":1,"fix":%s}}\n' "$(jstr "$1")" "$(jstr "$2")" "$(jstr "$3")"
	else
		echo "$2" >&2
		echo "  fix: $3" >&2
	fi
	exit 1
}

if [ "$provider" != cloudflare ]; then
	fail unknown_provider "preview.provider is $provider, and this version of the layer knows only cloudflare" \
		"set it back with: ilk config set preview.provider cloudflare"
fi

# The branch: the argument, else what CI says it is building, else git's.
branch=${1:-${WORKERS_CI_BRANCH:-${GITHUB_HEAD_REF:-}}}
if [ -z "$branch" ]; then
	branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)
fi
if [ -z "$branch" ] || [ "$branch" = HEAD ]; then
	fail no_branch "no branch to name a preview after (detached HEAD, and no WORKERS_CI_BRANCH or GITHUB_HEAD_REF)" \
		"pass the branch: ilk preview url <branch>"
fi

flat=$(mktemp)
out=$(mktemp)
trap 'rm -f "$flat" "$out"' EXIT
if ! cfg=$(sh "$here/ilk-preview-config.sh" path 2>"$flat"); then
	fail no_config "$(cat "$flat")" "add the Worker's wrangler.jsonc, or point preview.wrangler_config at it"
fi
sh "$here/ilk-preview-config.sh" flat >"$flat"
get() {
	awk -F '\t' -v k="$1" '$1 == k { print $2; exit }' "$flat"
}

worker=${ILK_VAR_WORKER_NAME:-auto}
if [ "$worker" = auto ] || [ -z "$worker" ]; then
	worker=$(get name)
fi
if [ -z "$worker" ]; then
	fail no_worker_name "$cfg has no name, so there is no Worker to preview" \
		"add \"name\" to $cfg, or set preview.worker_name"
fi

# A preview alias is lowercase letters, digits and dashes, starts with a letter,
# and with the Worker's name has to fit one DNS label: <alias>-<worker> is at
# most 63 characters. Truncating keeps long branch names usable; two branches
# that agree on their first characters will share an alias, which is why the
# alias is printed with the URL.
alias=$(printf '%s' "$branch" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g; s/--*/-/g; s/^-//; s/-$//')
case $alias in
[a-z]*) ;;
*) alias="br-$alias" ;;
esac
room=$((63 - ${#worker} - 1))
if [ "$room" -lt 1 ]; then
	fail worker_name_too_long "the Worker name $worker leaves no room for a preview alias in a 63-character hostname label" \
		"shorten the Worker's name"
fi
alias=$(printf '%s' "$alias" | cut -c "1-$room" | sed 's/-$//')

url=
if [ "$upload" = 1 ]; then
	# Uploading is CI's job: a version built on a laptop carries whatever is in
	# that working tree, and nobody can say afterwards what it was.
	if [ -z "${CI:-}" ] && [ -z "${WORKERS_CI:-}" ]; then
		fail not_in_ci "--upload runs only in CI (CI or WORKERS_CI set); a preview built on a laptop is a preview of an unknown tree" \
			"let the pull request's build upload it (Cloudflare Workers Builds or the CI pipeline), then run ilk preview url without --upload"
	fi
	set -- versions upload --preview-alias "$alias"
	if [ "${ILK_VAR_WRANGLER_CONFIG:-auto}" != auto ]; then
		set -- "$@" --config "$cfg"
	fi
	# shellcheck disable=SC2086 # the wrangler variable is a command line, e.g. `npx wrangler`
	if ! ${ILK_VAR_WRANGLER:-npx wrangler} "$@" >"$out" 2>&1; then
		cat "$out" >&2
		fail upload_failed "wrangler versions upload failed; its output is above" \
			"read Wrangler's error. A branch that adds, renames or deletes a Durable Object class cannot be uploaded as a version at all"
	fi
	cat "$out" >&2
	# Wrangler prints the alias URL and the version URL; the alias is the stable one.
	url=$(grep -i 'alias url' "$out" | grep -o 'https://[^ ]*' | head -1 || true)
	[ -n "$url" ] || url=$(grep -i 'preview url' "$out" | grep -o 'https://[^ ]*' | head -1 || true)
	if [ -z "$url" ]; then
		fail no_preview_url "the version uploaded but Wrangler reported no preview URL" \
			"run ilk check --only preview: preview_urls must be true, and a Worker that implements a Durable Object gets no preview URL at all"
	fi
else
	subdomain=${ILK_VAR_WORKERS_SUBDOMAIN:-}
	if [ -z "$subdomain" ]; then
		fail no_subdomain "the preview URL needs the account's workers.dev subdomain, and preview.workers_subdomain is not set" \
			"ilk config set preview.workers_subdomain <name> — the <name> in <worker>.<name>.workers.dev; it is not a secret"
	fi
	url="https://$alias-$worker.${subdomain%.workers.dev}.workers.dev"
fi

uploaded=false
[ "$upload" = 1 ] && uploaded=true
if [ "$json" = 1 ]; then
	printf '{"url":%s,"alias":%s,"provider":"cloudflare","worker":%s,"branch":%s,"uploaded":%s}\n' \
		"$(jstr "$url")" "$(jstr "$alias")" "$(jstr "$worker")" "$(jstr "$branch")" "$uploaded"
else
	echo "$url"
fi
