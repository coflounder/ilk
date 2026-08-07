---
id: plan-layer-queue
title: Layer queue
status: active
updated: 2026-08-07
---

# Layer queue

What gets built next, in order, and why that order. Shapes live in
[layers/README.md](../../layers/README.md); this document is only the sequencing and
the reasoning behind it.

## Outcome

Every layer ilk publishes proves its own checks catch what it claims, and the four
backlog items currently blocked on core decisions are unblocked by making those
decisions rather than by working around them.

## The finding this queue answers

Twelve layers ship today. Nine enforce something, two enforce nothing, one runs
only what the repository hands it. That distribution is fine.

What is not fine: **ten of the twelve have no test that their checks fire.**
`ilk layer test` proves exactly one property — add is idempotent and rm restores
the repository — and CI runs it against all twelve. It never asserts that a check
rejects anything. Only `toolkit` and `gh-projects` ship a `test/run.sh`, and both
of those test provider scripts rather than checks.

The checks sit on top of Go builtins that *are* tested (`internal/checks` has real
coverage), so they probably work. But "the builtin is tested" and "this layer
wired it up correctly" are different claims, and only the first has evidence. A
layer whose check silently matches nothing looks exactly like a layer whose check
passes — the same failure mode `record.coverage` exists to catch in documents, and
ilk does not currently apply it to itself.

Every layer added before that is fixed multiplies unverified surface. Every layer
added after ships with evidence. That is the whole argument for the order below.

## Order

### 0 — Check assertions in `ilk layer test` · next

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
every layer that registers a check has at least one failing and one passing case.

### 1 — The credential story · unblocks two layers

Where a provider token comes from, and what the failure looks like when it is
absent. It must come from the environment, never the repository, and an absent
credential must say so rather than looking like an empty diff or a clean preview.

`gh-projects` sidestepped this by leaning on `gh` being already authenticated.
`pulumi` and `linear-mirror` cannot.

*Accepted when:* a layer requiring a credential skips its check with a reason
`ilk doctor` can print, and never reports success on a credential it did not have.

### 2 — The `mcp:` neutral artifact · unblocks two more

Add `mcp:` alongside `instructions:`, `skills:` and `hooks:` in the manifest, and
teach each target to render it. Roughly 150 lines of core plus per-target
rendering. This is the last artifact type where a layer would otherwise have to
ship one agent's literal file, which is the thing ilk exists not to do.

*Accepted when:* a layer declares an MCP server once and `.mcp.json` and
`.cursor/mcp.json` are both projections of it.

### 3 — Layers, in this order

| Layer | Why here |
|---|---|
| `pr-prep` | Smallest unblocked item, immediate value, no dependencies |
| `autoresearch` | Unblocked; the only layer measuring staleness by expiry rather than coupling |
| `deprecation` | Same mechanic as staleness-by-coupling, applied to removal dates |
| `secrets` | Highest-consequence agent failure; wraps an existing scanner |
| `pulumi` | After the credential story |
| `migrations` | After `secrets`, which establishes the wrap-a-tool-via-capability shape |
| `codegraph` | After `mcp:`, for the version worth having |
| `linear-mirror` | After the credential story |
| `release-notes` | After `pr-prep`, which it shares its derive-from-the-record idea with |
| `html-wireframe` | Pairs with `visual-qa`; lower value than the above |
| `incident` | Only meaningful for a project with production |

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
