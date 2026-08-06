---
date: 2026-08-06
title: First working version — M0 and M1
---

# First working version — M0 and M1

Built the reconciliation engine and the layers that make `ilk init` useful on its own.

**What landed.** The fence engine and four ownership modes; `.ilk/config.yaml` and
`.ilk/lock.json`; layer resolution from built-in, local and git sources; the agent
target adapters; the check runner; `ilk brief`; and the CLI. Two built-in layers:
`ilk/record` and `ilk/quality-gates`.

**The proof point.** `internal/engine/engine_test.go` asserts the contract the whole
design rests on — adopt, edit around, upgrade and drop leaves a repository exactly as
it was. Getting there surfaced three real defects: a merge-mode file recorded an empty
provenance hash and so reported itself as drifted on every run; removing consecutive
fenced regions left a growing gap of blank lines; and an emptied co-owned file was left
behind as a husk rather than deleted.

**Dogfooding found the fourth.** Running `ilk init` on ilk's own repository failed
`record.naming` and `record.frontmatter` against `docs/PROPOSAL.md` and
`docs/LAYERS.md`. The check was right — they were renamed to `REF-` documents with
frontmatter. Worth noting that this is the collision every adopter with an existing
`docs/` directory will hit; the `docs_dir` variable exists for it, but the default
still surprises.

**Not yet built.** The registry and `ilk search`; three-way merges on upgrade
(conflicts are currently reported and skipped, never merged); `--pure` mode; adapters
beyond the ones listed in `ilk agents list`.
