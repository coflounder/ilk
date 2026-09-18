#!/bin/sh
set -eu
export PYTHONDONTWRITEBYTECODE=1
suite=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
build=$(mktemp -d)
trap 'rm -rf "$build"' EXIT HUP INT TERM
(cd "$suite/../../.." && go build -o "$build/ilk" ./cmd/ilk)
export ILK_TEST_BIN="$build/ilk"
python3 -m unittest discover -s "$suite" -p 'test_*.py' -v
uv run --script "$suite/protocol.py"
