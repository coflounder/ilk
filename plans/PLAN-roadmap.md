---
id: plan-roadmap
title: Roadmap
status: active
updated: 2026-08-06
---

# Roadmap

## Outcome

A repository can adopt a published practice with one command, upgrade it deliberately,
and drop it cleanly — for any coding agent, with the bare minimum working out of the
box.

## Boundaries

Not in scope: project tracking (Linear, Jira, GitHub Projects sync), org-level
configuration across repositories, an MCP server, Windows support for layer commands
and hooks. Integrations belong in layers, not in the core.

## Decisions already made

- [Fenced regions over whole files](../docs/DEC-fenced-regions-over-whole-files.md)
- Project-scoped, monorepo-only. One repository, one `.ilk/`.
- Opinionated defaults over configurability; iterate once the defaults hurt.
- Go single static binary. See [system overview](../docs/ARCH-system-overview.md).

## Slices

### M0 — Reversibility · done

The engine, the fence, four ownership modes, plan/apply, lockfile provenance.

*Accepted when:* adopt → edit around → upgrade → drop leaves the repository as it was,
asserted by test rather than by inspection. ✓

### M1 — Useful alone · done

`init`, `check`, `brief`, `adopt`/`drop`/`plan`/`apply`/`status`/`doctor`, the `record`
and `quality-gates` layers, AGENTS.md and git-hook projection, `--json` everywhere.

*Accepted when:* `ilk init` in a repository with no network access produces something
that validates, and the repository is better to work in than it was. ✓

### M2 — Extensible · partly done

Layer SDK (`layer new` / `validate` / `test`), capability resolution, subcommand
dispatch, local and git sources, lockfile digests. ✓

Three-way merge on upgrade, with `--merge-markers`, `--accept` and `--force` as the
ways out of a genuine collision. Adoption baselines, so a repository with an existing
`docs/` is not greeted by a wall of failures. ✓

Remaining:

- `--pure` mode: adopt with a hard guarantee that nothing executes.
- Version constraints on `requires:`, so a layer can need a capability *and* a version.

*Accepted when:* somebody outside this repository publishes a layer, and adopting it
needs no changes to ilk.

### M3 — Ecosystem · partly done

A layer index with `ilk search`, embedded so discovery works offline. Three community
layers in `layers/`, each reducing a MetaHarness principle to plain markdown and git,
and each held to `ilk layer test` in CI. An `ilk/toolkit` layer so an adopter's agent
can operate and extend ilk from a base `init`. ✓

Remaining:

- `ilk publish`, and moving the index out of this repository once there is enough in
  it to warrant its own.
- More adapters, with conformance fixtures so "works with any agent" stops being a
  claim that decays silently as agents change their formats.
- More layers. Candidates, in rough order of demand: `ralph` (a bounded agent loop
  whose completion condition is a check rather than the model's opinion), `llm-wiki`
  (a maintained knowledge layer over the codebase), `pr-prep`, `incident`.

*Accepted when:* the registry has enough layers that `ilk search` returns something
useful for a common need, and none of them required a core change.

### M4 — Hardening

Provenance verification, exec-consent audit, a docs site, and `ilk upgrade --dry-run`
diffs that show content rather than just operations.

## Open questions

1. **Is a commit count the right unit for staleness?** It ignores size: ten typo
   fixes and one rewrite score the same. Lines changed, or a diff against the covered
   paths, might discriminate better — at the cost of a threshold nobody can reason
   about. Worth revisiting once somebody has lived with the current one.
2. **Should the baseline expire?** Files exempted at adoption stay exempt until
   somebody clears them. That is deliberate, but a repository could sit on a hundred
   grandfathered files for ever and believe it is conformant. A `ilk baseline list`
   nag in `ilk brief`, or a ceiling like the instruction budget, might be right.
3. **Should layers be able to depend on a *version* of a capability?** `test.command`
   says nothing about what the command guarantees.
4. **How should a layer ship a check that needs a real parser** for a language ilk knows
   nothing about, without every layer shipping a binary?
