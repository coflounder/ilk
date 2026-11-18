#!/bin/sh
# Open a preview the way an agent is allowed to: one GET, through Cloudflare
# Access, with the Access service token.
#
#   ilk preview open <url> [--body]
#
# The token's id and secret are read from the environment variables the layer
# names (CF_ACCESS_CLIENT_ID and CF_ACCESS_CLIENT_SECRET by default). The secret
# reaches curl on its standard input, never on its command line, so it is not
# in the process list; it is never printed, logged or written to a file.
#
# Only GET. A preview runs against production data unless it was given data of
# its own, so this command has no way to write.
#
# The token goes only to this Worker's own preview hosts (and any host listed in
# preview.token_hosts), only over https, and never follows a redirect: a
# redirect would carry the token's headers to wherever it pointed.
#
# Exit: 0 the preview answered 2xx; 1 it did not, or the token is missing;
# 2 a usage mistake.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

url=${1:-}
body_wanted=${ILK_FLAG_BODY:-0}
json=0
[ "${ILK_OUTPUT:-text}" = json ] && json=1

jstr() {
	printf '"%s"' "$(printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g')"
}

fail() {
	# fail <exit> <code> <message> <fix>
	if [ "$json" = 1 ]; then
		printf '{"error":{"code":%s,"message":%s,"exit":%s,"fix":%s}}\n' "$(jstr "$2")" "$(jstr "$3")" "$1" "$(jstr "$4")"
	else
		echo "$3" >&2
		echo "  fix: $4" >&2
	fi
	exit "$1"
}

if [ "${ILK_VAR_PROVIDER:-cloudflare}" != cloudflare ]; then
	fail 2 unknown_provider "preview.provider is ${ILK_VAR_PROVIDER}, and this version of the layer knows only cloudflare" \
		"ilk config set preview.provider cloudflare"
fi

case $url in
https://*) ;;
*) fail 2 not_https "the URL must start with https:// — the service token is never sent in the clear" \
	"pass the preview's https URL, as ilk preview url prints it" ;;
esac

# The host, read from the address itself: no userinfo, no port, no path.
authority=${url#https://}
authority=${authority%%/*}
authority=${authority%%\?*}
authority=${authority%%#*}
case $authority in
*@*) fail 2 bad_url "the URL carries a user name, which is how an address hides where it really goes" \
	"pass the plain preview URL" ;;
esac
host=$(printf '%s' "${authority%%:*}" | tr '[:upper:]' '[:lower:]')

# The token goes to this Worker's own preview hosts and nowhere else. Every
# version and alias URL is <prefix>-<worker>.<subdomain>.workers.dev, so the
# match needs both names: "anything on workers.dev" would hand the token to any
# Worker anybody can create. Hosts in preview.token_hosts are added on purpose.
here=$(cd "$(dirname "$0")" && pwd)
worker=${ILK_VAR_WORKER_NAME:-auto}
if [ "$worker" = auto ] || [ -z "$worker" ]; then
	worker=$(sh "$here/ilk-preview-config.sh" flat 2>/dev/null | awk -F '\t' '$1 == "name" { print $2; exit }' || true)
fi
subdomain=${ILK_VAR_WORKERS_SUBDOMAIN:-}
subdomain=$(printf '%s' "${subdomain%.workers.dev}" | tr '[:upper:]' '[:lower:]')
worker=$(printf '%s' "$worker" | tr '[:upper:]' '[:lower:]')

allowed=0
if [ -n "$worker" ] && [ -n "$subdomain" ]; then
	case $host in
	?*-"$worker.$subdomain.workers.dev") allowed=1 ;;
	esac
fi
for extra in $(printf '%s' "${ILK_VAR_TOKEN_HOSTS:-}" | tr ',' ' '); do
	case $host in
	"$extra" | *."$extra") allowed=1 ;;
	esac
done
if [ "$allowed" = 0 ]; then
	if [ -z "$subdomain" ]; then
		fail 2 host_not_allowed "$host is not a host the service token may go to: preview.workers_subdomain is not set, so this Worker's preview hosts cannot be told from anybody else's on workers.dev" \
			"ilk config set preview.workers_subdomain <name> — the <name> in <worker>.<name>.workers.dev"
	fi
	fail 2 host_not_allowed "$host is not one of this Worker's preview hosts (*-$worker.$subdomain.workers.dev) or in preview.token_hosts, so the service token is not sent there" \
		"open the URL ilk preview url prints; if $host is genuinely this Worker's, add it with ilk config set preview.token_hosts $host"
fi

id_name=${ILK_VAR_CLIENT_ID_ENV:-CF_ACCESS_CLIENT_ID}
secret_name=${ILK_VAR_CLIENT_SECRET_ENV:-CF_ACCESS_CLIENT_SECRET}
for n in "$id_name" "$secret_name"; do
	case $n in
	"" | [0-9]* | *[!A-Za-z0-9_]*)
		fail 2 bad_env_name "$n is not an environment variable name" \
			"set preview.client_id_env and preview.client_secret_env to names like CF_ACCESS_CLIENT_ID"
		;;
	esac
done
eval "id_val=\${$id_name-}"
eval "secret_val=\${$secret_name-}"
missing=
[ -n "$id_val" ] || missing="$id_name"
[ -n "$secret_val" ] || missing="${missing:+$missing and }$secret_name"
if [ -n "$missing" ]; then
	fail 1 token_missing "$missing not set in the environment, so there is no Access service token to open the preview with" \
		"export them from wherever the token is kept (a secret store, CI secrets) — never from a file in this repository, and never paste the secret into a chat or a log"
fi

body=$(mktemp)
heads=$(mktemp)
trap 'rm -f "$body" "$heads"' EXIT

# printf is a shell builtin, so the secret travels down a pipe and never
# appears as an argument of any process.
status=0
code=$(printf 'CF-Access-Client-Id: %s\nCF-Access-Client-Secret: %s\n' "$id_val" "$secret_val" |
	curl -sS --proto '=https' --max-time 30 --max-redirs 0 -H @- \
		-o "$body" -D "$heads" -w '%{http_code}' -- "$url") || status=$?
unset secret_val id_val

if [ "$status" -ne 0 ]; then
	fail 1 unreachable "curl could not fetch $url (exit $status)" \
		"check the URL with ilk preview url, and that the preview's build has finished"
fi

location=$(grep -i '^location:' "$heads" | tail -1 | sed 's/^[Ll][Oo][Cc][Aa][Tt][Ii][Oo][Nn]:[[:space:]]*//' | tr -d '\r')
loc_host=
if [ -n "$location" ]; then
	loc_host=${location#*://}
	loc_host=${loc_host%%/*}
fi
access_login=false
case $loc_host in
*.cloudflareaccess.com) access_login=true ;;
esac

ok=false
case $code in
2??) ok=true ;;
esac

if [ "$json" = 1 ]; then
	printf '{"url":%s,"status":%s,"ok":%s,"access_login":%s,"redirect_host":%s}\n' \
		"$(jstr "$url")" "$code" "$ok" "$access_login" "$(jstr "$loc_host")"
else
	echo "$code $url" >&2
	if [ "$access_login" = true ]; then
		echo "Access sent the request to its sign-in page: it did not accept the service token." >&2
		echo "  fix: the preview's Access app needs a Service Auth policy that includes this token, and the token must not have expired" >&2
	elif [ -n "$loc_host" ]; then
		echo "redirected to $loc_host, not followed (the token is not sent on)" >&2
	elif [ "$code" = 401 ] || [ "$code" = 403 ]; then
		echo "refused: Access let the request through but the Worker would not answer it, or Access rejected the token outright" >&2
	elif [ "$code" = 404 ] && grep -q '1042' "$body" 2>/dev/null; then
		echo "error 1042: there is no preview at this address — a Worker that implements a Durable Object gets no preview URL" >&2
	fi
	if [ "$body_wanted" = 1 ]; then
		cat "$body"
	fi
fi

[ "$ok" = true ]
