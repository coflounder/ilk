---
date: 2026-08-06
title: Adoption baselines, and three-way merge on upgrade
---

# Adoption baselines, and three-way merge on upgrade

Closed the two gaps the first working version left open.

## Adopting into a repository with history

`ilk init` used to fail immediately in any repository that already had a `docs/`
folder: the naming grammar and frontmatter checks judged files nobody had touched.
This repository hit it on its own first run.

The fix is not a different default directory — it is that **a layer governs what
happens next, not what came before**. Files already present when a layer's directory
contract arrives are recorded as its baseline and exempted from that layer's checks.
Files added afterwards are governed normally.

It is a ratchet, not an amnesty: recorded in the lockfile, listed by
`ilk baseline list`, and tightened one file at a time with `ilk baseline clear`.
`--no-baseline` opts into strictness from the start. Solving it in the core rather
than in the record layer matters, because any layer that governs an existing
directory has the same problem.

## Three-way merge

Upgrades over a file somebody had edited were a hard refusal. Usually the two
changes are disjoint — the layer rewrote one section, the user edited another — and
refusing every such case makes upgrades painful enough that people stop running them.

This needed a new piece of state. The lockfile stored a hash, which detects that a
file changed but cannot reconcile two changes to it, so ilk now keeps a copy of what
it last wrote under `.ilk/base/`, content-addressed and committed.

The merge itself is deliberately conservative: disjoint or identical changes merge,
anything else is reported with the conflicting line numbers. A wrong merge is worse
than a refusal, because a refusal is visible. Out of a genuine collision there are
three ways: `--merge-markers` writes both versions into the file, `--accept` keeps
yours as the new baseline, `--force` takes the layer's.

## Two bugs found by writing the tests

**The lockfile conflated two different facts.** One hash was doing duty as both
"what ilk expects the file to be" and "what the layer last delivered". Those diverge
the moment a merge succeeds — and the next `ilk apply` then read the merged file as
"unchanged since ilk wrote it" and overwrote it, silently destroying the merge it had
just performed. Split into `Hash` and `Delivered`; `reconcileArtifact` is now the
single place the decision is made.

**`lock.Find` ignored the owner.** It matched on path and region alone, so two layers
each owning an `instructions` region in `AGENTS.md` — which is the normal case —
resolved to whichever was recorded first. Latent before, load-bearing now that
provenance drives merging.

Both were caught by tests written to assert the intended behaviour rather than to
cover the code, which is the argument for writing them that way.

## Also

`--merge-markers` leaves a file that reads as instructions to an agent and as noise
to a human, so `ilk check` gained `ilk.conflicts` to catch unresolved markers, and
`ilk.drift` now stays quiet about those files rather than reporting the same problem
with a worse message.
