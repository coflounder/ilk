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

# Pin the harness APIs exercised by adapter tests. Install only into this test's
# disposable directory; never change the developer's harness configuration.
npm install --prefix "$build/harnesses" --ignore-scripts --no-audit --no-fund @earendil-works/pi-coding-agent@0.85.1 @openai/codex@0.153.4 typescript@5.9.3 @types/node@22.10.0 >/dev/null
export ILK_TEST_PI_ROOT="$build/harnesses/node_modules/@earendil-works/pi-coding-agent"
export ILK_TEST_CODEX_BIN="$build/harnesses/node_modules/.bin/codex"
python3 - "$suite" "$build" <<'PYCODE'
import json, os, sys
from pathlib import Path
suite, build = map(Path, sys.argv[1:])
sdk = Path(os.environ["ILK_TEST_PI_ROOT"])
config = {"compilerOptions": {"noEmit": True, "strict": True, "skipLibCheck": True,
    "module": "ESNext", "moduleResolution": "Bundler", "target": "ES2022", "types": ["node"],
    "typeRoots": [str(build / "harnesses/node_modules/@types")],
    "paths": {"@earendil-works/pi-coding-agent": [str(sdk / "dist/index.d.ts")]}},
    "files": [str(suite.parents[2] / "internal/targets/pi/extension.ts"), str(suite / "pi-template.d.ts")]}
(build / "tsconfig.json").write_text(json.dumps(config))
PYCODE
"$build/harnesses/node_modules/.bin/tsc" -p "$build/tsconfig.json"
node "$suite/pi.mjs"
python3 "$suite/codex.py"
