# ilk

<!--
  Instructions for coding agents working in this repository.

  Prose outside the ilk:begin / ilk:end blocks is yours: describe what this project
  is, how to build and test it, and the non-obvious things a newcomer gets wrong.
  Keep it short and specific — restating what the repository already shows measurably
  makes agents worse, not better.

  The fenced blocks below are generated from the layers in .ilk/config.yaml.
-->

<!-- ilk:begin layer=ilk/quality-gates region=instructions — managed by ilk — edits inside this block are overwritten; run `ilk drop` to remove it -->
Work is not done because it looks done. Before claiming a task is complete, run
`ilk check` and let it decide. It runs this repository's tests, linter and build,
and every failure it prints comes with the fix.

The same gates run on `git push` and in CI, so a claim that cannot survive them will
not survive review either. Never disable a gate, mark a test skipped, or weaken an
assertion to make a check pass — if a gate is genuinely wrong, say so plainly and
leave it failing rather than quietly removing the thing that would have caught you.

When you report that work is finished, state what you ran and what it said. "Tests
pass" with no output behind it is a claim, not evidence.
<!-- ilk:end layer=ilk/quality-gates region=instructions -->

<!-- ilk:begin layer=ilk/record region=instructions — managed by ilk — edits inside this block are overwritten; run `ilk drop` to remove it -->
This repository keeps a **project record** next to the code. Agents cannot inherit
what was never written down, so anything a future session would need to know goes
in a file, not in a chat.

| Directory | Holds | Filename grammar |
|---|---|---|
| `docs/` | What is true now | `ARCH-system-overview.md` — uppercase type, kebab slug |
| `plans/` | What we intend to build, and what "accepted" means | `PLAN-billing-rewrite.md` |
| `log/` | What happened, dated | `2026-08-06-shipped-billing.md` |
| `scratch/` | Rough notes. Ungoverned, unchecked, gitignored | anything |

Documents in `docs/` and `plans/` carry YAML frontmatter
with `id`, `title`, `status`, `updated`. Create them with `ilk record new <dir> <title>`
so the naming and frontmatter are right the first time.

Rules that matter:

- **Put rough thinking in `scratch/`.** It exists so the other three stay
  clean. A governed system needs an ungoverned annex, or the governance leaks into
  the canonical documents.
- **Never edit inside an `ilk:begin` / `ilk:end` block.** Those lines are generated;
  your edits there are overwritten on the next `ilk apply`.
- **When you change what is true, update `docs/` in the same change.**
  A doc that lags the code becomes a failing `ilk check`, not a quiet surprise later.

Verify your own work before claiming it is done: `ilk check` validates the record and
prints the fix for anything it rejects. `ilk brief` prints the current state of the
project. Both accept `--json`.
<!-- ilk:end layer=ilk/record region=instructions -->

<!-- ilk:begin layer=target:agents-md region=skills — managed by ilk — edits inside this block are overwritten; run `ilk drop` to remove it -->
## Skills

Detailed procedures live in files, not in this document. Read the matching
file when its situation applies — do not read them all up front.

- **prove-it** — Turn a claim that work is done into evidence a reviewer can check. Use before marking a task complete, opening a pull request, or reporting a result.
  `.agent/skills/prove-it/SKILL.md`
- **review-changes** — Review a change the way this repository expects, before it is proposed. Use when preparing a pull request or when asked to review a diff.
  `.agent/skills/review-changes/SKILL.md`
- **update-record** — Bring the project record back in line with the code after a change. Use after shipping work that alters architecture, interfaces, or operational behaviour, or when `ilk check` reports a stale document.
  `.agent/skills/update-record/SKILL.md`
- **write-decision** — Record an architectural or process decision so future sessions inherit it. Use when a choice is made that a later reader would otherwise have to reverse-engineer, or would plausibly undo by accident.
  `.agent/skills/write-decision/SKILL.md`
<!-- ilk:end layer=target:agents-md region=skills -->
