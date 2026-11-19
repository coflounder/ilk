#!/bin/sh
# The preview layer's checks, one aspect per invocation.
#
#   ilk-preview-check.sh <aspect>
#
# Aspects: workers-dev-off, urls-on, workers-previews, previews-complete,
# access-verified, preview-audience, separate-data, no-committed-token.
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
	"") finding "$cfg" "preview_urls is not set, so it follows workers_dev, and a Preview gets no workers.dev URL while workers_dev is off" ;;
	*) finding "$cfg" "preview_urls is $v, so a Preview gets no workers.dev URL and nobody can open it" ;;
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
	# A Workers Preview reads its vars from the previews block alone, so the
	# audience has to be there too, or every preview refuses its own tokens.
	if has_top previews && [ -z "$(get "previews.vars.$name")" ]; then
		finding "$cfg" "previews.vars has no $name, and a Preview takes its vars from the previews block alone, so every preview refuses tokens from its own Access app"
	fi
	worker_sources
	if ! src_grep "$name" >/dev/null; then
		finding "$worker_dir" "no source under $worker_dir reads $name, so a token minted for the preview Access app is never accepted"
	fi
	done_
	;;

workers-previews)
	need_config
	# Workers Previews (`wrangler preview`, configured by a `previews` block)
	# are how Cloudflare says to preview a branch or a pull request: each gets
	# its own URL, only the bindings the block gives it, and its own namespace
	# for a Durable Object class this Worker implements. Without the block, the
	# only previews left are uploaded versions, which Cloudflare says not to use
	# for branch or pull request testing: they run with production's bindings,
	# and a Worker that implements a Durable Object gets no URL at all.
	if has_top previews; then
		done_
	fi
	finding "$cfg" "there is no previews block, so pull requests can only be previewed as uploaded versions, which run with production's bindings and which Cloudflare says not to use for branch or pull request testing"
	# A binding without script_name is a class this Worker implements; with one,
	# it is somebody else's Worker. Name the classes, because for this Worker
	# versions are not a fallback: they get no URL.
	awk -F '\t' '
		match($1, /^durable_objects\.bindings\[[0-9]+\]\./) {
			i = substr($1, 1, RLENGTH - 1)
			field = substr($1, RLENGTH + 1)
			if (field == "class_name") cls[i] = $2
			if (field == "script_name" && $2 != "") ext[i] = 1
		}
		END { for (i in cls) if (!(i in ext)) print cls[i] }
	' "$flat" | sort -u | while read -r cls; do
		finding "$cfg" "this Worker implements the Durable Object $cls, and Cloudflare makes no Version URL for a Worker that implements one (it answers error 1042), so without Workers Previews it has no previews at all"
	done
	done_
	;;

previews-complete)
	need_config
	# Nothing to compare without a previews block; preview.workers-previews
	# reports that.
	if ! has_top previews; then
		done_
	fi
	# A Preview inherits no bindings and no vars from the top level. Every one
	# production has must be given again in the previews block — with its own
	# store where it holds data (preview.separate-data judges that) — or code
	# that reads it gets undefined on a Preview, and a missing Durable Object
	# binding answers error 1101. Routes, crons, migrations, assets and
	# compatibility settings are production's alone and are not compared.
	awk -F '\t' '
		{
			side = "prod"; p = $1
			if (index(p, "previews.") == 1) { side = "prev"; p = substr(p, 10) }
			what = ""
			if (p ~ /^vars\.[^.[]+$/) what = "var " substr(p, 6)
			else if (p ~ /^(kv_namespaces|d1_databases|r2_buckets|services|hyperdrive|vectorize|analytics_engine_datasets|workflows)\[[0-9]+\]\.binding$/ ||
				p ~ /^queues\.producers\[[0-9]+\]\.binding$/ ||
				p ~ /^durable_objects\.bindings\[[0-9]+\]\.name$/ ||
				p ~ /^send_email\[[0-9]+\]\.name$/ ||
				p ~ /^(ai|browser|images|version_metadata)\.binding$/) {
				kind = p; sub(/[.[].*/, "", kind)
				what = "binding " $2 " (" kind ")"
			}
			if (what == "") next
			have[side SUBSEP what] = 1
			if (side == "prod") prod[what] = 1
		}
		END { for (w in prod) if (!(("prev" SUBSEP w) in have)) print w }
	' "$flat" | sort -u | while read -r what; do
		finding "$cfg" "production has the $what and the previews block does not; a Preview inherits nothing from the top level, so on a Preview it is undefined (a missing Durable Object binding answers error 1101)"
	done | grep . && exit 1
	done_
	;;

separate-data)
	if [ "${ILK_VAR_SHARED_BINDINGS:-refuse}" = read-only ]; then
		# Accepted on purpose, in .ilk/config.yaml, by somebody who read the fix.
		done_
	fi
	need_config
	state="kv_namespaces d1_databases r2_buckets durable_objects queues hyperdrive vectorize workflows services"
	if has_top previews; then
		# Workers Previews inherit no bindings: only what the `previews` block
		# binds reaches a preview. A Durable Object class this Worker implements
		# gets a namespace per preview, so it starts empty and is not
		# production's. A binding into another Worker's Durable Object is that
		# Worker's data. Anything else is shared when it names the same store
		# — KV id, database, bucket, queue, index, service — production binds.
		awk -F '\t' -v kinds="$state" '
			BEGIN {
				n = split(kinds, k, " "); for (i = 1; i <= n; i++) want[k[i]] = 1
				split("id database_id bucket_name queue index_name service script_name", f, " ")
				for (i in f) ident[f[i]] = 1
			}
			{
				side = "prod"; p = $1
				if (index(p, "previews.") == 1) { side = "prev"; p = substr(p, 10) }
				split(p, part, /[.[]/)
				if (!(part[1] in want)) next
				if (!match(p, /^[a-z_]+(\.bindings)?\[[0-9]+\]/)) next
				b = substr(p, 1, RLENGTH); field = substr(p, RLENGTH + 2)
				if (side == "prod") { if (field in ident) seen[part[1] SUBSEP field SUBSEP $2] = 1; next }
				kind[b] = part[1]
				if (field == "name" || field == "binding") name[b] = $2
				if (field == "script_name") ext[b] = $2
				if (field in ident) { idf[b] = field; idv[b] = $2 }
			}
			END {
				for (b in kind) {
					label = (b in name) ? name[b] : b
					if (kind[b] == "durable_objects") {
						if (b in ext) print kind[b] "\t" label ", a Durable Object of the Worker " ext[b] ", whose data production reads too"
						continue
					}
					if (!(b in idf)) print kind[b] "\t" label ", and nothing in it says which store, so it is taken to be production'"'"'s"
					else if ((kind[b] SUBSEP idf[b] SUBSEP idv[b]) in seen) print kind[b] "\t" label ", the same " idf[b] " production binds"
				}
			}
		' "$flat" | sort -u | while IFS='	' read -r b what; do
			finding "$cfg" "previews.$b binds $what — a write in a preview is a write to production's data"
		done | grep . && exit 1
		done_
	fi
	# Bindings that hold or reach state. Workers AI, assets and plain vars do not.
	for b in $state; do
		if has_top "$b"; then
			finding "$cfg" "$b is bound at the top level and there is no previews block, so every preview is an uploaded version that runs with the same $b as production"
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
	echo "usage: ilk-preview-check.sh workers-dev-off|urls-on|workers-previews|previews-complete|access-verified|preview-audience|separate-data|no-committed-token" >&2
	exit 2
	;;
esac
