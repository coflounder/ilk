# ilk

<!--
  Instructions for coding agents working in this repository.

  Prose outside the ilk:begin / ilk:end blocks is yours: describe what this project
  is, how to build and test it, and the non-obvious things a newcomer gets wrong.
  Keep it short and specific — restating what the repository already shows measurably
  makes agents worse, not better.

  The fenced blocks below are generated from the layers in .ilk/config.yaml.
-->

`ilk` manages the process layers a repository uses to work well with coding agents.
It is a Go binary with no runtime dependencies. `docs/ARCH-system-overview.md` has the
package map; read it before changing anything that crosses a package boundary.

Non-obvious things that catch people out here:

- **The built-in layers are embedded, not read from disk.** Editing
  `internal/builtin/layers/**` only takes effect after `go build` — a stale binary
  silently uses the old templates.
- **`internal/engine/engine_test.go` is the contract, not a test suite.** It asserts
  that adopt → edit around → upgrade → drop leaves a repository as it was. If a change
  makes one of those tests fail, the change is almost certainly wrong; loosening the
  assertion is not the fix.
- **Two functions decide correctness for the whole tool**: `planOne` in
  `internal/engine/plan.go`, and `Upsert`/`Remove` in `internal/fence`. Everything else
  is presentation.
- **Ownership modes are not interchangeable.** Before adding an artifact, decide who
  owns the file. Choosing `managed` for a file a human might also write in is the
  single most damaging mistake available in this codebase.
- **Every check must carry a `fix`.** The manifest will not validate without one, and
  a fix that does not tell the reader what to actually do defeats the point.
- CI runs `ilk layer test` against both built-in layers. They are held to the same
  contract as any published layer.

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

Create documents with `ilk record new <dir> <title>` so the naming and frontmatter
are right the first time. Documents in `docs/` also declare `covers:` —
the paths they describe. Staleness is measured against those paths rather than the
calendar, so a document goes stale exactly when its subject changes.

Rules that matter:

- **Put rough thinking in `scratch/`.** It exists so the other three stay
  clean. A governed system needs an ungoverned annex, or the governance leaks.
- **Never edit inside an `ilk:begin` / `ilk:end` block.** Those lines are generated.
- **If your change touches a path some document `covers:`, that document is yours
  this session.** Update it in the same change, or run `ilk record review <file>` to
  see what moved and record that you read it.

Verify before claiming done: `ilk check` validates the record and prints the fix for
anything it rejects. `ilk brief` prints the current state. Both accept `--json`.
<!-- ilk:end layer=ilk/record region=instructions -->

<!-- ilk:begin layer=target:agents-md region=skills — managed by ilk — edits inside this block are overwritten; run `ilk drop` to remove it -->
## Skills

Detailed procedures live in files, not in this document. Read the matching
file when its situation applies — do not read them all up front.

- **adopt-a-layer** — Evaluate a layer and add it to this repository. Use when asked to adopt, install or try an ilk layer, when someone links to one, or when deciding whether a published layer is worth taking on.
  `.agent/skills/adopt-a-layer/SKILL.md`
- **apply-ilk-changes** — Reconcile the repository with its declared layers. Use when `ilk status` reports drift, after editing .ilk/config.yaml by hand, when `ilk check` reports ilk.drift, or when upgrading a layer to a new version.
  `.agent/skills/apply-ilk-changes/SKILL.md`
- **compound-a-lesson** — Turn something that went wrong, or went well, into a change that alters default behaviour for every future session. Use after an incident, a repeated review comment, a near-miss, or when the same mistake has now been made twice.
  `.agent/skills/compound-a-lesson/SKILL.md`
- **configure-ilk** — Change what ilk does in this repository — capabilities, agent targets, layer variables, budgets and check exemptions. Use when a check is skipped for want of a capability, when adding support for another coding agent, or when a layer's defaults do not fit.
  `.agent/skills/configure-ilk/SKILL.md`
- **prove-it** — Turn a claim that work is done into evidence a reviewer can check. Use before marking a task complete, opening a pull request, or reporting a result.
  `.agent/skills/prove-it/SKILL.md`
- **resolve-an-ilk-conflict** — Decide what to do when ilk refuses to write a file because it was edited after ilk wrote it. Use when a plan shows CONFLICT, when a merge reports conflicting hunks, or when `ilk check` reports ilk.conflicts.
  `.agent/skills/resolve-an-ilk-conflict/SKILL.md`
- **review-changes** — Review a change the way this repository expects, before it is proposed. Use when preparing a pull request or when asked to review a diff.
  `.agent/skills/review-changes/SKILL.md`
- **update-record** — Bring the project record back in line with the code after a change. Use after shipping work that alters architecture, interfaces, or operational behaviour, or when `ilk check` reports a stale document.
  `.agent/skills/update-record/SKILL.md`
- **write-a-layer** — Author a new ilk layer, or change an existing one. Use when a practice worth repeating should become installable — a directory contract, a check, a skill, a hook or a subcommand — or when asked to extend what ilk does.
  `.agent/skills/write-a-layer/SKILL.md`
- **write-decision** — Record an architectural or process decision so future sessions inherit it. Use when a choice is made that a later reader would otherwise have to reverse-engineer, or would plausibly undo by accident.
  `.agent/skills/write-decision/SKILL.md`
<!-- ilk:end layer=target:agents-md region=skills -->

<!-- ilk:begin layer=ilk/compound-lessons region=instructions — managed by ilk — edits inside this block are overwritten; run `ilk drop` to remove it -->
When something goes wrong twice, or a review comment repeats, the fix is not to
remember harder. Encode it: a new check, a new skill, a convention written down, or a
change to a layer. Advice that depends on everybody hearing and remembering it does
not survive the next joiner or the next session.

Record it as `docs/LESSON-*.md`, and say in its frontmatter what
durable change it produced. `ilk check` will not accept a lesson that changed nothing.
<!-- ilk:end layer=ilk/compound-lessons region=instructions -->
