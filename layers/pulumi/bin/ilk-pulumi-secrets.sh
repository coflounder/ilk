#!/bin/sh
# A secret in a stack config file is either encrypted or it is in your git history.
#
# `pulumi config set --secret` writes the value as a `secure:` mapping, which is
# safe to commit. Everything else — a password typed straight into
# Pulumi.prod.yaml because the deploy was failing and it was late — is a plaintext
# credential that no amount of deleting it later takes back out of the history.
#
# Detection is by key name, because that is the only signal a file can carry: a
# key whose name reads as a credential must resolve to `secure:`. The project file
# is scanned as well as the per-stack files; a secret is no less exposed there.
#
# Managed by ilk — edits are overwritten on the next `ilk apply`.
set -eu

group=${ILK_VAR_GROUP:-infra}

# Nothing to say about a group nobody has created yet.
[ -d "$group" ] || exit 0

report=$(find "$group" -type f \( -name Pulumi.yaml -o -name "Pulumi.*.yaml" \) -exec awk '
function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
function flush() {
	if (pending != "") {
		print pendingfile ":" pendingline ": `" pending "` reads as a credential and has no `secure:` value under it"
		pending = ""
	}
}
FNR == 1 { flush() }
{
	line = $0
	sub(/[ \t]*#.*$/, "", line)

	# A mapping line is `key: value`, or `key:` with the value nested beneath.
	# Splitting on the first colon-space rather than the first colon is what keeps
	# a Pulumi key like `myproject:dbPassword` in one piece.
	key = ""
	value = ""
	p = index(line, ": ")
	if (p > 0) {
		key = trim(substr(line, 1, p - 1))
		value = trim(substr(line, p + 2))
	} else if (line ~ /:[ \t]*$/) {
		sub(/:[ \t]*$/, "", line)
		key = trim(line)
	} else {
		next
	}
	gsub(/"/, "", key)

	# The encrypted form is a credential key with the value on the next line, so
	# a `secure:` immediately after one answers it. Anything else does not.
	if (tolower(key) == "secure") { pending = ""; next }
	flush()

	# `secretsprovider` names the thing that does the encrypting. It is not itself
	# a secret, and failing it would teach people to exempt the whole check.
	if (tolower(key) ~ /secretsprovider|secrets_provider|secrets-provider/) next
	if (tolower(key) !~ /secret|password|passwd|token|apikey|api_key|api-key|privatekey|private_key|private-key|passphrase|credential/) next

	if (value == "") {
		pending = key
		pendingline = FNR
		pendingfile = FILENAME
		next
	}
	if (value ~ /secure:/) next
	print FILENAME ":" FNR ": `" key "` holds a plaintext value"
}
END { flush() }
' {} +)

if [ -n "$report" ]; then
	printf "%s\n" "$report"
	exit 1
fi
