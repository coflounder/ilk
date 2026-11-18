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
# Terraform and OpenTofu have no encrypted form, so for them a `.tfvars` file
# that git would commit must not give a credential-named variable a value at
# all. The value belongs in the environment (`TF_VAR_<name>`) or in a .tfvars
# file that .gitignore keeps out, with a committed `.tfvars.example` beside it.
# A .tfvars file git ignores is not read: it never reaches the history.
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

# The .tfvars files git would commit: tracked, or new and not ignored. Outside a
# git repository there is nothing to tell them apart, so every one is read.
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	tfvars=$(git ls-files --cached --others --exclude-standard -- "$group" | grep -E '\.tfvars$' || true)
else
	tfvars=$(find "$group" -type f -name '*.tfvars')
fi

if [ -n "$tfvars" ]; then
	tfreport=$(printf "%s\n" "$tfvars" | while IFS= read -r f; do
		[ -f "$f" ] && printf "%s\0" "$f"
	done | xargs -0 awk '
function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s); return s }
{
	line = $0
	# `#` and `//` start comments in HCL; a value containing either is rare in a
	# credential and not worth a parser for.
	sub(/[ \t]*(#|\/\/).*$/, "", line)
	p = index(line, "=")
	if (p == 0) next
	key = trim(substr(line, 1, p - 1))
	value = trim(substr(line, p + 1))
	gsub(/"/, "", key)
	if (tolower(key) !~ /secret|password|passwd|token|apikey|api_key|api-key|privatekey|private_key|private-key|passphrase|credential/) next
	# An empty string is the placeholder a committed example is meant to carry.
	if (value == "" || value == "\"\"" || value == "null") next
	print FILENAME ":" FNR ": `" key "` holds a plaintext value in a .tfvars file git would commit"
}
')
	if [ -n "$tfreport" ]; then
		report=$(printf "%s\n%s" "$report" "$tfreport" | sed '/^$/d')
	fi
fi

if [ -n "$report" ]; then
	printf "%s\n" "$report"
	exit 1
fi
