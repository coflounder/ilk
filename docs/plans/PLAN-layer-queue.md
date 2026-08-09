---
id: plan-layer-queue
title: Layer queue
status: active
updated: 2026-08-09
---

# Layer queue

What gets built next, in order, and why that order. Shapes live in
[layers/README.md](../../layers/README.md); this document is only the sequencing and
the reasoning behind it.

## Outcome

Every layer ilk publishes proves its own checks catch what it claims, and the four
backlog items currently blocked on core decisions are unblocked by making those
decisions rather than by working around them.

## The finding this queue answered

Twelve layers shipped when this was written. Nine enforced something, two enforced
nothing, one ran only what the repository handed it. That distribution was fine.

What was not fine: **ten of the twelve had no test that their checks fire.**
`ilk layer test` proves exactly one property — add is idempotent and rm restores
the repository — and CI runs it against all twelve. It never asserts that a check
rejects anything. Only `toolkit` and `gh-projects` ship a `test/run.sh`, and both
of those test provider scripts rather than checks.

The checks sat on top of Go builtins that *are* tested (`internal/checks` has real
coverage), so they probably worked. But "the builtin is tested" and "this layer
wired it up correctly" are different claims, and only the first had evidence. A
layer whose check silently matches nothing looks exactly like a layer whose check
passes — the same failure mode `record.coverage` exists to catch in documents, and
ilk was not applying it to itself.

Every layer added before that was fixed multiplied unverified surface; every layer
added after ships with evidence. That was the whole argument for the order below,
and it held: the eight layers that followed carry 63 assertions between them, and
two of the first three found real bugs the moment they ran.

## Order

### 0 — Check assertions in `ilk layer test` · done

Extend the layer test harness so a layer can assert its checks reject what they
are supposed to reject, then backfill the ten layers that have none.

The sandbox already exists — `ilk layer test` builds a throwaway repository, adds
the layer, applies twice and removes it. The missing piece is planting a fixture
that violates a check and asserting the check fails:

```yaml
# layers/blueprint/test/checks.yaml
- check: blueprint.epic
  fixture: spec-with-missing-epic/
  expect: fail
- check: blueprint.epic
  fixture: spec-with-real-epic/
  expect: pass
```

Both directions matter. A check that fails on everything is as broken as one that
fails on nothing, and only the second is currently possible to notice.

*Accepted when:* CI fails if a layer's check stops rejecting its own fixture, and
every layer that registers a check has at least one failing and one passing case. ✓
The harness landed with `--strict` for the second half, and it found `blueprint.epic`
passing on a spec whose epic does not exist — the sandbox had been stubbing required
capabilities to invented paths, so the check looked somewhere empty.

### 1 — The credential story · done

Where a provider token comes from, and what the failure looks like when it is
absent. It must come from the environment, never the repository, and an absent
credential must say so rather than looking like an empty diff or a clean preview.

`gh-projects` sidestepped this by leaning on `gh` being already authenticated.
`pulumi` and `linear-mirror` cannot.

*Accepted when:* a layer requiring a credential skips its check with a reason
`ilk doctor` can print, and never reports success on a credential it did not have. ✓
`requires_env:` names the variables, tests them for presence and never reads them.
`pulumi` is its first tenant; `linear-mirror` needs nothing further from core.

### 2 — The `mcp:` neutral artifact · next

Add `mcp:` alongside `instructions:`, `skills:` and `hooks:` in the manifest, and
teach each target to render it. Roughly 150 lines of core plus per-target
rendering. This is the last artifact type where a layer would otherwise have to
ship one agent's literal file, which is the thing ilk exists not to do.

*Accepted when:* a layer declares an MCP server once and `.mcp.json` and
`.cursor/mcp.json` are both projections of it.

### 3 — Layers, in this order

| Layer | Why here | |
|---|---|---|
| `pr-prep` | Smallest unblocked item, immediate value, no dependencies | done |
| `autoresearch` | Unblocked; the only layer measuring staleness by expiry rather than coupling | done |
| `deprecation` | Same mechanic as staleness-by-coupling, applied to removal dates | done |
| `secrets` | Highest-consequence agent failure; wraps an existing scanner | done |
| `pulumi` | After the credential story | done |
| `routine` | Not on this list when it was written: the trigger half of running unattended | done |
| `gauntlet` | Nor was this one: the judging half, and the reason unattended work can be trusted | done |
| `html-wireframe` | Pairs with `visual-qa`; was ranked lower, and was cheap once the pair was the argument | done |
| `migrations` | After `secrets`, which establishes the wrap-a-tool-via-capability shape | |
| `codegraph` | After `mcp:`, for the version worth having | |
| `linear-mirror` | Unblocked; three shell commands over `ilk mirror` | |
| `release-notes` | After `pr-prep`, which it shares its derive-from-the-record idea with | |
| `incident` | Only meaningful for a project with production | |

`routine` and `gauntlet` were not on the original list. They came out of asking what a
repository needs in order to run while nobody is watching it — something to start the
work, and something other than the builder to decide it is good enough — and that
question turned out to be worth more than the next two items in the ranking.

## Boundaries

Not in this queue, and each for a stated reason:

- **`brainstorm`** — argued out of existing. A check counting alternatives is
  cheapest to satisfy by writing strawmen, and the durable artifact already exists
  as a decision naming what it rejected.
- **`kanban`** — a competing planning model rather than an addition. Worth building
  only as a view over `blueprint`, owning no state.
- **`llm-wiki`** — overlaps `codegraph` and the record. Revisit once `codegraph`
  exists, since the interesting version is the synthesis over the index.
- **`pm-loops`** — shipped as `plan-hygiene`, with the scheduling half deliberately
  dropped.

## Open questions

1. **Should check assertions be mandatory?** CI could fail any layer in this
   repository that registers a check without a fixture. That is the right standard
   here; whether to hold *published* layers to it is a question about how much
   friction the ecosystem will bear before people stop publishing.
2. **Does `secrets` belong in ilk at all?** It wraps a scanner, and a repository
   could wire that scanner into `gates` directly with no layer. The layer earns its
   place only if it adds the pre-commit discipline and the "what to do when one
   leaks" skill, not merely the check.
3. **Is `migrations` too stack-specific?** The hash-an-applied-migration rule is
   general; where migrations live and what "applied" means is not. It may need a
   capability supplying the migrations directory and the applied-set, which is a
   larger surface than any current layer requires.
