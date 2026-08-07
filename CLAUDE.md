<!-- ilk:begin layer=target:claude-code region=pointer — managed by ilk — edits inside this block are overwritten; run `ilk rm` to remove it -->
This project keeps its agent instructions in **AGENTS.md** at the repository root.
Read that file — it is the source of truth for Claude Code and every other agent working here.

The project's machine interface is the `ilk` command:

- `ilk brief` — the current state of the project, assembled from the record.
- `ilk check` — validate the repository; every failure prints its own fix.

Both accept `--json`.
<!-- ilk:end layer=target:claude-code region=pointer -->
