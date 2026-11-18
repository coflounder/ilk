#!/bin/sh
# The preview layer's checks, one aspect per invocation.
#
#   ilk-preview-check.sh <aspect>
#
# Aspects: workers-dev-off, urls-on, access-verified, preview-audience,
# no-durable-objects, separate-data, no-committed-token.
#
# Each prints its findings as JSON lines (`findings: jsonl` in layer.yaml) and
# exits 0 when the repository is right, 1 when it is not, and 2 when the check
# itself could not run. Everything is read from files: no check here calls
# Cloudflare, runs wrangler, or needs a credential.
#
# The heuristics are deliberately simple. They read the Wrangler configuration
# through ilk-preview-config.sh and grep the Worker's source; a check that
# guesses wrong says so in its finding, so a person can see why it fired.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
aspect=${1:-}
provider=${ILK_VAR_PROVIDER:-cloudflare}

if [ "$provider" != cloudflare ]; then
	echo "preview.provider is $provider; this version of the layer checks only cloudflare" >&2
	exit 2
fi

# A JSON string, quotes included. Messages are ilk's own words plus names read
# from the configuration, so escaping quotes and backslashes is all it takes.
jstr() {
	printf '"%s"' "$(printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g' | tr '\t\n' '  ')"
}

found=0
finding() {
	# finding <path> <message>
	found=1
	if [ -n "$1" ]; then
		printf '{"path":%s,"message":%s}\n' "$(jstr "$1")" "$(jstr "$2")"
	else
		printf '{"message":%s}\n' "$(jstr "$2")"
	fi
}

done_() {
	exit "$found"
}

flat=$(mktemp)
srcs=$(mktemp)
trap 'rm -f "$flat" "$srcs"' EXIT

# Every check but the committed-token one is about the configuration, so a
# repository without one fails them all with the same plain finding.
need_config() {
	if ! cfg=$(sh "$here/ilk-preview-config.sh" path 2>"$flat"); then
		finding "" "$(cat "$flat") — the preview layer reads the Worker's Wrangler configuration to check it"
		done_
	fi
	sh "$here/ilk-preview-config.sh" flat >"$flat"
}

# The value at an exact path, or nothing.
get() {
	awk -F '\t' -v k="$1" '$1 == k { print $2; exit }' "$flat"
}

# Whether any value sits at or under a top-level key.
has_top() {
	awk -F '\t' -v k="$1" '
		$1 == k || index($1, k ".") == 1 || index($1, k "[") == 1 { hit = 1; exit }
		END { exit !hit }
	' "$flat"
}

# The Worker's own source files, tests left out: a test that mentions the Access
# header proves nothing about the Worker that answers requests.
worker_sources() {
	dir=${ILK_VAR_WORKER_SRC:-auto}
	if [ "$dir" = auto ] || [ -z "$dir" ]; then
		main=$(get main)
		if [ -n "$main" ]; then
			dir=$(dirname "$main")
		else
			dir=src
		fi
	fi
	if [ ! -d "$dir" ]; then
		finding "$cfg" "the Worker's source directory $dir does not exist (set preview.worker_src to where it lives)"
		done_
	fi
	find "$dir" \( -name node_modules -o -name .git -o -name .wrangler -o -name __tests__ \) -prune -o \
		-type f \( -name '*.ts' -o -name '*.mts' -o -name '*.tsx' -o -name '*.js' -o -name '*.mjs' -o -name '*.cjs' -o -name '*.jsx' \) \
		! -name '*.test.*' ! -name '*.spec.*' ! -name '*.d.ts' -print >"$srcs"
	worker_dir=$dir
}

# grep across the Worker's sources; prints the matching files.
src_grep() {
	[ -s "$srcs" ] || return 1
	tr '\n' '\0' <"$srcs" | xargs -0 grep -l -i -E -- "$1" /dev/null 2>/dev/null
}

case $aspect in
workers-dev-off)
	need_config
	v=$(get workers_dev)
	case $v in
	false) ;;
	"") finding "$cfg" "workers_dev is not set, and Wrangler turns the public workers.dev route on when it is absent" ;;
	*) finding "$cfg" "workers_dev is $v, so this Worker answers on a public workers.dev address" ;;
	esac
	done_
	;;

urls-on)
	need_config
	v=$(get preview_urls)
	case $v in
	true) ;;
	"") finding "$cfg" "preview_urls is not set, so whether uploaded versions get a preview URL is left to Wrangler's default" ;;
	*) finding "$cfg" "preview_urls is $v, so an uploaded version gets no preview URL" ;;
	esac
	done_
	;;

access-verified)
	need_config
	worker_sources
	header=$(src_grep 'cf-access-jwt-assertion' || true)
	if [ -z "$header" ]; then
		finding "$worker_dir" "no source under $worker_dir reads the cf-access-jwt-assertion header, so the Worker trusts that every request came through Cloudflare Access"
		done_
	fi
	# Reading the header is not verifying it. Look for the signature check: a
	# JWT library's verify, WebCrypto's verify, or the team's certs endpoint.
	verify=$(src_grep 'jwtVerify|verifyJwt|verifyJWT|jwt\.verify|crypto\.subtle\.verify|/cdn-cgi/access/certs' || true)
	if [ -z "$verify" ]; then
		first=$(echo "$header" | head -1)
		finding "$first" "reads cf-access-jwt-assertion but nothing under $worker_dir verifies its signature (no jwtVerify, crypto.subtle.verify or /cdn-cgi/access/certs)"
	fi
	done_
	;;

preview-audience)
	need_config
	name=${ILK_VAR_PREVIEW_AUDIENCE_VAR:-ACCESS_AUD_PREVIEW}
	v=$(get "vars.$name")
	if [ -z "$v" ]; then
		finding "$cfg" "vars has no $name, so the Worker has no audience to accept tokens from the Access app on its preview URLs"
	fi
	worker_sources
	if ! src_grep "$name" >/dev/null; then
		finding "$worker_dir" "no source under $worker_dir reads $name, so a token minted for the preview Access app is never accepted"
	fi
	done_
	;;

no-durable-objects)
	need_config
	# A binding without script_name is a class this Worker implements; with one,
	# it is somebody else's Worker and costs this one nothing.
	awk -F '\t' '
		match($1, /^durable_objects\.bindings\[[0-9]+\]\./) {
			i = substr($1, 1, RLENGTH - 1)
			field = substr($1, RLENGTH + 1)
			if (field == "class_name") cls[i] = $2
			if (field == "script_name" && $2 != "") ext[i] = 1
		}
		END { for (i in cls) if (!(i in ext)) print cls[i] }
	' "$flat" | sort -u | while read -r cls; do
		finding "$cfg" "this Worker implements the Durable Object $cls, and Cloudflare makes no preview URL for a Worker that implements one (the URL answers error 1042)"
	done | grep . && exit 1
	done_
	;;

separate-data)
	if [ "${ILK_VAR_SHARED_BINDINGS:-refuse}" = read-only ]; then
		# Accepted on purpose, in .ilk/config.yaml, by somebody who read the fix.
		done_
	fi
	need_config
	# Bindings that hold or reach state. Workers AI, assets and plain vars do not.
	for b in kv_namespaces d1_databases r2_buckets durable_objects queues hyperdrive vectorize workflows services; do
		if has_top "$b"; then
			finding "$cfg" "$b is bound at the top level, and every preview version runs with the same $b as production"
		fi
	done
	done_
	;;

no-committed-token)
	name=${ILK_VAR_CLIENT_SECRET_ENV:-CF_ACCESS_CLIENT_SECRET}
	if cfg=$(sh "$here/ilk-preview-config.sh" path 2>/dev/null); then
		sh "$here/ilk-preview-config.sh" flat >"$flat"
		if [ -n "$(get "vars.$name")" ]; then
			finding "$cfg" "vars carries $name; vars are committed in plain text and shipped to the Worker, so the service token's secret is in the repository"
		fi
	fi
	# Tracked files when there is history, so a gitignored .dev.vars holding
	# the secret for local use is not a finding. Without history there is no
	# telling what is ignored, so local env files are left out rather than
	# reported.
	list=$(mktemp)
	if git rev-parse --git-dir >/dev/null 2>&1; then
		git ls-files >"$list"
	else
		find . \( -name .git -o -name node_modules -o -name .wrangler -o -name .ilk \) -prune -o \
			-type f ! -name '.dev.vars*' ! -name '.env*' -print | sed 's|^\./||' >"$list"
	fi
	# The secret is 64 hex characters. A name followed by a long hex value is
	# an assignment; a name followed by a CI secrets expression or $X is a reference.
	pattern="($name|CF-Access-Client-Secret)[\"']?[[:space:]]*[:=][[:space:]]*[\"']?[A-Fa-f0-9]{32,}"
	hits=
	if [ -s "$list" ]; then
		hits=$(tr '\n' '\0' <"$list" | xargs -0 grep -I -n -E -- "$pattern" /dev/null 2>/dev/null || true)
	fi
	rm -f "$list"
	if [ -n "$hits" ]; then
		echo "$hits" | cut -d: -f1,2 | sort -u | while IFS=: read -r file line; do
			printf '{"path":%s,"line":%s,"message":%s}\n' "$(jstr "$file")" "$line" \
				"$(jstr "assigns a value to $name — an Access service token secret is committed")"
		done
		exit 1
	fi
	done_
	;;

*)
	echo "usage: ilk-preview-check.sh workers-dev-off|urls-on|access-verified|preview-audience|no-durable-objects|separate-data|no-committed-token" >&2
	exit 2
	;;
esac
