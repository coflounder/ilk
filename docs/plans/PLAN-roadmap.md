---
id: plan-roadmap
title: Roadmap
status: active
updated: 2026-08-08
---

# Roadmap

## Outcome

A repository can adopt a published practice with one command, upgrade it deliberately,
and drop it cleanly — for any coding agent, with the bare minimum working out of the
box.

## Boundaries

Not in scope: org-level configuration across repositories, an MCP server, Windows
support for layer commands and hooks. Integrations belong in layers, not in the core.

Tracker sync was on this list and came off it deliberately. What core gained is
`ilk mirror` — identity, diffing, refusing on ambiguity, plan-then-apply — and no
knowledge of any provider; a layer supplies three commands that normalise a tracker to
`{id, title, status, url}`. The boundary held where it mattered: `gh-projects` knows what
a GitHub Project is, and nothing in the binary does.

## Decisions already made

- [Fenced regions over whole files](../reference/DEC-fenced-regions-over-whole-files.md)
- [Groups, not ordinals, for directory layout](../reference/DEC-groups-not-ordinals.md)
- Project-scoped, monorepo-only. One repository, one `.ilk/`.
- Opinionated defaults over configurability; iterate once the defaults hurt.
- Go single static binary. See [system overview](../reference/ARCH-system-overview.md).

## Slices

### M0 — Reversibility · done

The engine, the fence, four ownership modes, plan/apply, lockfile provenance.

*Accepted when:* adopt → edit around → upgrade → drop leaves the repository as it was,
asserted by test rather than by inspection. ✓

### M1 — Useful alone · done

`init`, `check`, `brief`, `add`/`rm`/`plan`/`apply`/`status`/`doctor`, the `record`
and `gates` layers, AGENTS.md and git-hook projection, `--json` everywhere.

*Accepted when:* `ilk init` in a repository with no network access produces something
that validates, and the repository is better to work in than it was. ✓

### M2 — Extensible · partly done

Layer SDK (`layer new` / `validate` / `test`), capability resolution, subcommand
dispatch, local and git sources, lockfile digests. ✓

Three-way merge on upgrade, with `--merge-markers`, `--accept` and `--force` as the
ways out of a genuine collision. Adoption baselines, so a repository with an existing
`docs/` is not greeted by a wall of failures. ✓

`ilk contribute` and the `maintainer` layer close the loop the other way: a repository
that has tuned a layer can send back what it learned, with the divergence, its history
and the friction all gathered from state ilk already keeps, and the receiving end has
checks that fail a proposal nobody can judge. ✓

Remaining:

- `--pure` mode: add a layer with a hard guarantee that nothing executes.
- **`ilk:` in a manifest is parsed and never enforced.** A layer declaring
  `ilk: ">=0.2.0"` is making a promise nothing checks, which is worse than not having the
  field: a repository on an older binary gets a confusing render failure instead of
  "this layer needs a newer ilk". Enforce it or drop it.
- Version constraints on `requires:`, so a layer can need a capability *and* a version.

*Accepted when:* somebody outside this repository publishes a layer, and adopting it
needs no changes to ilk.

### M2.5 — Standard surface · done

The pass before the first external consumer, where breaking things is still free.
`add`/`rm` in place of `adopt`/`drop`; `.ilk/lock.json` keyed by `owners` with an explicit
`kind`, since it holds layers, core and target adapters alike; `.agents/skills/` matching
the path the wider tooling uses; per-skill symlinks instead of copied skill bodies; and
directory groups so the repository root stops growing one folder per layer.

*Accepted when:* a repository written against these names does not have to be rewritten
when somebody else starts using ilk. ✓

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
- More layers: `codegraph`, `migrations`, `release-notes`, `incident`, `linear-mirror`,
  in the order set out in [PLAN-layer-queue](PLAN-layer-queue.md) — **deferred** in
  favour of documentation and onboarding; see
  [PLAN-docs-onboarding](PLAN-docs-onboarding.md). The backlog with
  decided shapes is in [layers/README.md](../../layers/README.md). `brainstorm` was
  specified and then argued out of existing, on the grounds that a check counting
  alternatives is cheapest to satisfy with strawmen.

The `mcp:` neutral artifact — previously the clearest remaining core gap — is done.
A layer declares a server once and `.mcp.json` and `.cursor/mcp.json` are both
co-owned projections of it, each entry an `ilk mcp run <name>` indirection with
`requires_env:` carrying the credential story. That unblocks `mcp-servers` and the
version of `codegraph` worth having.

The queue's first two items are done. Check assertions closed the gap it was written
about — ten of twelve layers had no evidence their checks rejected anything — and the
credential story is decided: `requires_env:` on a command check names the variables that
carry a credential, tests them for presence without ever reading them, and skips with a
reason rather than failing as though the thing it checks were empty. `pulumi` is its
first tenant. That was `linear-mirror`'s last blocker as well as its own.

Eight layers landed against that: `secrets`, `pulumi`, `deprecation`, `pr-prep`,
`autoresearch`, and then the three that make a repository able to run while nobody is
watching it — `routine`, which holds scheduled work as documents and projects them into
whatever actually runs them; `gauntlet`, which refuses to call work done until somebody
who did not build it has compared it against something inspectable; and
`html-wireframe`, which moves the argument about an interface to the day it costs an
afternoon rather than a build. None needed a core change, which was the point of the
milestone.

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
5. **Should ilk help move files when a directory contract moves?** `ilk apply` creates the
   new directory and removes what it owns, but it will not move a document a human wrote,
   so the grouped-layout migration was a manual `git mv` plus a link sweep. A
   `ilk layout move` that planned renames the way everything else plans writes is
   plausible; whether relocating somebody's files is ilk's business at all is the real
   question.
6. **Should the built-in layers live in `layers/` with the rest?** They are ordinary
   layers held to the same contract, and only sit under `internal/builtin/` because
   `go:embed` cannot read above its own package. A generated copy at build time would
   unify the tree at the cost of a step that silently ships stale templates when skipped.
