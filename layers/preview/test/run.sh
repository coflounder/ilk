#!/bin/sh
# Exercise the preview layer's commands against a fake wrangler and a fake curl.
#
# What this proves: the contract with ilk (ILK_VAR_*, ILK_FLAG_*, ILK_OUTPUT=json),
# that the name is derived the same way every time, that --upload only ever runs
# `wrangler preview --name` (or, with no previews block, `wrangler versions
# upload`) and only in CI, and that `open` keeps the service
# token's secret off every command line, refuses hosts that are not this
# Worker's, and fails plainly when the token is unset. What it cannot prove is
# that Cloudflare answers the way the fakes do; only a real account can do that,
# and nothing here talks to one.
#
#   sh layers/preview/test/run.sh
set -eu

here=$(cd "$(dirname "$0")" && pwd)
bin=$here/../bin
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

PATH=$here/fake:$PATH
FAKE_LOG=$work/fake.log
export PATH FAKE_LOG
: >"$FAKE_LOG"

# Run everything from a copy of the compliant fixture, as ilk would, from the
# repository root.
cp -R "$here/fixtures/compliant/." "$work/repo"
cd "$work/repo"

unset CI WORKERS_CI WORKERS_CI_BRANCH GITHUB_HEAD_REF CF_ACCESS_CLIENT_ID CF_ACCESS_CLIENT_SECRET || true
ILK_VAR_PROVIDER=cloudflare
ILK_VAR_WRANGLER_CONFIG=auto
ILK_VAR_WORKER_NAME=auto
ILK_VAR_WORKERS_SUBDOMAIN=acme
ILK_VAR_WRANGLER=wrangler
ILK_VAR_CLIENT_ID_ENV=CF_ACCESS_CLIENT_ID
ILK_VAR_CLIENT_SECRET_ENV=CF_ACCESS_CLIENT_SECRET
ILK_VAR_TOKEN_HOSTS=
ILK_FLAG_UPLOAD=0
ILK_FLAG_BODY=0
ILK_OUTPUT=text
export ILK_VAR_PROVIDER ILK_VAR_WRANGLER_CONFIG ILK_VAR_WORKER_NAME ILK_VAR_WORKERS_SUBDOMAIN \
	ILK_VAR_WRANGLER ILK_VAR_CLIENT_ID_ENV ILK_VAR_CLIENT_SECRET_ENV ILK_VAR_TOKEN_HOSTS \
	ILK_FLAG_UPLOAD ILK_FLAG_BODY ILK_OUTPUT

failures=0
pass() { echo "  ✓ $1"; }
fail() {
	echo "  ✗ $1"
	[ -n "${2:-}" ] && echo "      got: $2"
	failures=$((failures + 1))
}
same() {
	if [ "$2" = "$3" ]; then pass "$1"; else
		fail "$1" "$2"
		echo "      want: $3"
	fi
}

echo "preview provider: cloudflare"

# --- url -------------------------------------------------------------------

out=$(sh "$bin/ilk-preview-url.sh" feat/Checkout_Totals)
same "url derives the alias URL from branch, Worker and subdomain" \
	"$out" "https://feat-checkout-totals-shop.acme.workers.dev"

out=$(ILK_OUTPUT=json sh "$bin/ilk-preview-url.sh" feat/checkout)
same "url --json reports url, alias and provider" "$out" \
	'{"url":"https://feat-checkout-shop.acme.workers.dev","alias":"feat-checkout","provider":"cloudflare","worker":"shop","branch":"feat/checkout","uploaded":false}'

out=$(sh "$bin/ilk-preview-url.sh" 123-fix)
same "an alias always starts with a letter" "$out" "https://br-123-fix-shop.acme.workers.dev"

long=feature/an-extremely-long-branch-name-that-would-never-fit-in-one-dns-label-at-all
out=$(ILK_OUTPUT=json sh "$bin/ilk-preview-url.sh" "$long" | sed 's/.*"alias":"\([^"]*\)".*/\1/')
if [ $((${#out} + 5)) -le 63 ] && [ "${out%-}" = "$out" ]; then
	pass "a long branch is cut to fit <alias>-<worker> in one 63-character label"
else
	fail "a long branch is cut to fit <alias>-<worker> in one 63-character label" "$out"
fi

out=$(WORKERS_CI_BRANCH=fix/from-builds sh "$bin/ilk-preview-url.sh")
same "url names the branch Workers Builds is building" "$out" "https://fix-from-builds-shop.acme.workers.dev"

if out=$(ILK_VAR_WORKERS_SUBDOMAIN='' sh "$bin/ilk-preview-url.sh" main 2>&1); then
	fail "url without a subdomain refuses rather than guessing" "$out"
else
	case $out in *workers_subdomain*) pass "url without a subdomain refuses, and names the setting" ;; *) fail "url without a subdomain refuses, and names the setting" "$out" ;; esac
fi

# --- url --upload ------------------------------------------------------------

: >"$FAKE_LOG"
if out=$(ILK_FLAG_UPLOAD=1 sh "$bin/ilk-preview-url.sh" feat/x 2>&1); then
	fail "--upload refuses outside CI" "$out"
else
	if [ -s "$FAKE_LOG" ]; then fail "--upload refuses outside CI, before calling wrangler" "$(cat "$FAKE_LOG")"; else pass "--upload refuses outside CI, before calling wrangler"; fi
fi

: >"$FAKE_LOG"
out=$(CI=true ILK_FLAG_UPLOAD=1 sh "$bin/ilk-preview-url.sh" feat/x 2>/dev/null)
same "--upload reports the Preview URL wrangler printed" "$out" "https://feat-x-shop.acme.workers.dev"
same "--upload builds a Workers Preview with the worked-out name, and nothing else" "$(cat "$FAKE_LOG")" "preview --name feat-x"

out=$(CI=true ILK_FLAG_UPLOAD=1 sh "$bin/ilk-preview-url.sh" feat/x 2>/dev/null)
same "--upload names the Preview as url without --upload prints it" "$out" "$(sh "$bin/ilk-preview-url.sh" feat/x)"

out=$(CI=true ILK_FLAG_UPLOAD=1 ILK_OUTPUT=json sh "$bin/ilk-preview-url.sh" feat/x 2>/dev/null)
case $out in *'"uploaded":true'*) pass "--upload --json says it uploaded" ;; *) fail "--upload --json says it uploaded" "$out" ;; esac

if out=$(CI=true ILK_FLAG_UPLOAD=1 FAKE_WRANGLER=nourl sh "$bin/ilk-preview-url.sh" feat/x 2>&1); then
	fail "a build with no preview URL fails and points at the checks" "$out"
else
	case $out in *"ilk check --only preview"*) pass "a build with no preview URL fails and points at the checks" ;; *) fail "a build with no preview URL fails and points at the checks" "$out" ;; esac
fi

if out=$(CI=true ILK_FLAG_UPLOAD=1 FAKE_WRANGLER=fail sh "$bin/ilk-preview-url.sh" feat/x 2>&1); then
	fail "a failed build fails" "$out"
else
	pass "a failed build fails"
fi

: >"$FAKE_LOG"
CI=true ILK_FLAG_UPLOAD=1 ILK_VAR_WRANGLER_CONFIG=wrangler.jsonc sh "$bin/ilk-preview-url.sh" feat/x >/dev/null 2>&1
same "an explicit wrangler_config is passed to wrangler" "$(cat "$FAKE_LOG")" "preview --name feat-x --config wrangler.jsonc"

# With no previews block, --upload falls back to the older route, an uploaded
# version with a preview alias, and says on stderr that Cloudflare advises
# against it for pull requests.
cp -R "$here/fixtures/versions-only/." "$work/vo"
: >"$FAKE_LOG"
out=$(cd "$work/vo" && CI=true ILK_FLAG_UPLOAD=1 sh "$bin/ilk-preview-url.sh" feat/x 2>"$work/vo.err")
same "--upload without a previews block reports the alias URL" "$out" "https://feat-x-shop.acme.workers.dev"
same "--upload without a previews block runs versions upload with the alias, and nothing else" "$(cat "$FAKE_LOG")" "versions upload --preview-alias feat-x"
case $(cat "$work/vo.err") in *warning*"no previews block"*preview.workers-previews*) pass "--upload without a previews block warns and names the check" ;; *) fail "--upload without a previews block warns and names the check" "$(cat "$work/vo.err")" ;; esac

# --- open --------------------------------------------------------------------

secret=$(printf 'deadbeef%.0s' 1 2 3 4 5 6 7 8)
url=https://feat-x-shop.acme.workers.dev/api/health

if out=$(sh "$bin/ilk-preview-open.sh" "$url" 2>&1); then
	fail "open fails plainly when the token is unset" "$out"
else
	case $out in *CF_ACCESS_CLIENT_ID*CF_ACCESS_CLIENT_SECRET*"not set"*) pass "open fails plainly when the token is unset, naming both variables" ;; *) fail "open fails plainly when the token is unset, naming both variables" "$out" ;; esac
fi

CF_ACCESS_CLIENT_ID=fixture.access
CF_ACCESS_CLIENT_SECRET=$secret
export CF_ACCESS_CLIENT_ID CF_ACCESS_CLIENT_SECRET

: >"$FAKE_LOG.argv"
: >"$FAKE_LOG.stdin"
out=$(sh "$bin/ilk-preview-open.sh" "$url" 2>&1)
same "open reports the status of a preview that answered" "$out" "200 $url"
if grep -q "$secret" "$FAKE_LOG.argv"; then
	fail "the secret never appears on curl's command line" "$(cat "$FAKE_LOG.argv")"
else
	pass "the secret never appears on curl's command line"
fi
if grep -q "^CF-Access-Client-Secret: $secret" "$FAKE_LOG.stdin" && grep -q '^CF-Access-Client-Id: fixture.access' "$FAKE_LOG.stdin"; then
	pass "the token's headers reach curl on standard input"
else
	fail "the token's headers reach curl on standard input" "$(cat "$FAKE_LOG.stdin")"
fi
case $(cat "$FAKE_LOG.argv") in *--max-redirs\ 0*"--proto =https"* | *"--proto =https"*--max-redirs\ 0*) pass "open never follows a redirect and refuses plain http" ;; *) fail "open never follows a redirect and refuses plain http" "$(cat "$FAKE_LOG.argv")" ;; esac
if echo "$out" | grep -q "$secret"; then fail "open never prints the secret" "$out"; else pass "open never prints the secret"; fi

out=$(ILK_FLAG_BODY=1 sh "$bin/ilk-preview-open.sh" "$url" 2>/dev/null)
same "--body prints the response body" "$out" "<!doctype html><title>shop</title>"

out=$(ILK_OUTPUT=json sh "$bin/ilk-preview-open.sh" "$url")
same "open --json reports status and ok" "$out" \
	"{\"url\":\"$url\",\"status\":200,\"ok\":true,\"access_login\":false,\"redirect_host\":\"\"}"

if out=$(FAKE_CURL_STATUS=302 FAKE_CURL_LOCATION=https://acme.cloudflareaccess.com/cdn-cgi/access/login sh "$bin/ilk-preview-open.sh" "$url" 2>&1); then
	fail "a redirect to the Access sign-in page fails and says the token was refused" "$out"
else
	case $out in *"did not accept the service token"*) pass "a redirect to the Access sign-in page fails and says the token was refused" ;; *) fail "a redirect to the Access sign-in page fails and says the token was refused" "$out" ;; esac
fi

refuses_host() {
	: >"$FAKE_LOG.argv"
	if out=$(sh "$bin/ilk-preview-open.sh" "$2" 2>&1); then
		fail "$1" "$out"
	elif [ -s "$FAKE_LOG.argv" ]; then
		fail "$1 (curl was called)" "$(cat "$FAKE_LOG.argv")"
	else
		pass "$1"
	fi
}
refuses_host "open refuses another Worker on workers.dev" https://feat-x-evil.acme.workers.dev/
refuses_host "open refuses the same Worker name on another account" https://feat-x-shop.other.workers.dev/
refuses_host "open refuses a host that only ends like a preview host" https://feat-x-shop.acme.workers.dev.example.com/
refuses_host "open refuses a URL hiding its host behind a user name" https://feat-x-shop.acme.workers.dev@example.com/
refuses_host "open refuses plain http" http://feat-x-shop.acme.workers.dev/
ILK_VAR_WORKERS_SUBDOMAIN='' refuses_host "open refuses everything until the subdomain is set" "$url"

: >"$FAKE_LOG.argv"
out=$(ILK_VAR_TOKEN_HOSTS="shop.example.com" sh "$bin/ilk-preview-open.sh" https://shop.example.com/ 2>&1)
same "a host listed in token_hosts is allowed" "$out" "200 https://shop.example.com/"

# --- the accepted-on-purpose path of a check --------------------------------

cp -R "$here/fixtures/shared-production-data/." "$work/shared"
cd "$work/shared"
if sh "$bin/ilk-preview-check.sh" separate-data >/dev/null; then
	fail "separate-data fails while production's bindings are shared"
else
	pass "separate-data fails while production's bindings are shared"
fi
if ILK_VAR_SHARED_BINDINGS=read-only sh "$bin/ilk-preview-check.sh" separate-data >/dev/null; then
	pass "shared_bindings: read-only accepts sharing on purpose"
else
	fail "shared_bindings: read-only accepts sharing on purpose"
fi

echo
if [ "$failures" -gt 0 ]; then
	echo "$failures failure(s)"
	exit 1
fi
echo "all passed"
