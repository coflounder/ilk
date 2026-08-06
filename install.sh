#!/bin/sh
# Install ilk.
#
#   curl -fsSL https://raw.githubusercontent.com/coflounder/ilk/main/install.sh | sh
#
# Environment:
#   ILK_VERSION   version to install (default: latest)
#   ILK_INSTALL   install directory (default: first writable of /usr/local/bin,
#                 ~/.local/bin, ~/bin)
set -eu

REPO="coflounder/ilk"
VERSION="${ILK_VERSION:-latest}"

fail() {
	echo "ilk: $1" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || fail "$1 is required but not installed"
}

need uname
need tar

if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL "$1"; }
	fetch_to() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -qO- "$1"; }
	fetch_to() { wget -qO "$2" "$1"; }
else
	fail "curl or wget is required"
fi

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
linux | darwin) ;;
*) fail "unsupported operating system: $os (build from source with \`go install github.com/$REPO/cmd/ilk@latest\`)" ;;
esac

arch=$(uname -m)
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) fail "unsupported architecture: $arch" ;;
esac

if [ "$VERSION" = "latest" ]; then
	VERSION=$(fetch "https://api.github.com/repos/$REPO/releases/latest" |
		sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
		head -n 1)
	[ -n "$VERSION" ] || fail "could not determine the latest version; set ILK_VERSION"
fi

plain=${VERSION#v}
archive="ilk_${plain}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$VERSION"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "ilk: downloading $VERSION ($os/$arch)"
fetch_to "$base/$archive" "$tmp/$archive" || fail "could not download $base/$archive"

# Verify against the release checksums when a hashing tool is available. A missing
# tool is not fatal, but a mismatch always is.
if fetch_to "$base/checksums.txt" "$tmp/checksums.txt" 2>/dev/null; then
	expected=$(grep " $archive\$" "$tmp/checksums.txt" | awk '{print $1}' || true)
	if [ -n "$expected" ]; then
		if command -v sha256sum >/dev/null 2>&1; then
			actual=$(sha256sum "$tmp/$archive" | awk '{print $1}')
		elif command -v shasum >/dev/null 2>&1; then
			actual=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
		else
			actual=""
		fi
		if [ -n "$actual" ] && [ "$actual" != "$expected" ]; then
			fail "checksum mismatch for $archive — refusing to install"
		fi
	fi
fi

tar -xzf "$tmp/$archive" -C "$tmp"
[ -f "$tmp/ilk" ] || fail "the archive did not contain an ilk binary"
chmod +x "$tmp/ilk"

if [ -n "${ILK_INSTALL:-}" ]; then
	dest="$ILK_INSTALL"
else
	dest=""
	for candidate in /usr/local/bin "$HOME/.local/bin" "$HOME/bin"; do
		if [ -d "$candidate" ] && [ -w "$candidate" ]; then
			dest="$candidate"
			break
		fi
	done
	if [ -z "$dest" ]; then
		dest="$HOME/.local/bin"
		mkdir -p "$dest"
	fi
fi

mkdir -p "$dest"
mv "$tmp/ilk" "$dest/ilk"

echo "ilk: installed to $dest/ilk"

case ":$PATH:" in
*":$dest:"*) ;;
*)
	echo
	echo "ilk: $dest is not on your PATH. Add it:"
	echo "  export PATH=\"$dest:\$PATH\""
	;;
esac

echo
echo "Next: cd into a repository and run \`ilk init\`."
