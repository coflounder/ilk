---
id: arch-system-overview
title: System overview
status: current
updated: 2026-08-06
---

# System overview

`ilk` is a single Go binary. It reads a repository's declared layers, computes what the
repository should contain, compares that against what is on disk, and reconciles the
two. Everything else is detail.

## The pipeline

```
.ilk/config.yaml          desired state, declared
        │
        ▼
  engine.Load             resolve every adopted layer from its source
        │
        ▼
  engine.Desired          render layers + project through agent targets
        │                 → a flat list of artifacts, each with an owner and a mode
        ▼
  engine.Plan             compare against disk and .ilk/lock.json
        │                 → actions, including refusals
        ▼
  engine.Apply            execute, then rewrite the lockfile
```

`plan` and `apply` are separate because adopting a layer should never be a surprise.
Nothing is written until the whole blast radius has been shown.

## Packages

| Package | Responsibility |
|---|---|
| `internal/fence` | The ownership markers. Upsert, extract and remove a block inside a file somebody else owns, losslessly. |
| `internal/manifest` | The layer schema and its validation. |
| `internal/layer` | Resolving a layer reference to a manifest plus a file tree: built-in, local path, or git. |
| `internal/config` | `.ilk/config.yaml` — desired state, comment-preserving on save. |
| `internal/lock` | `.ilk/lock.json` — provenance hashes for everything ilk wrote. |
| `internal/basestore` | `.ilk/base/` — content-addressed copies of what ilk last wrote, so upgrades can merge rather than refuse. |
| `internal/merge` | Line-based three-way merge. Conservative by construction: merges disjoint or identical changes, refuses anything else. |
| `internal/render` | Templating. Deliberately small; layers should be readable by whoever is deciding to adopt them. |
| `internal/targets` | Agent adapters. Project neutral artifacts into what each agent reads. |
| `internal/engine` | Desired-state computation, planning, applying. |
| `internal/checks` | The validator runner and the built-in checks. |
| `internal/brief` | The session-start packet. |
| `internal/cli` | Command surface. |
| `internal/builtin` | Layers embedded in the binary, so `ilk init` needs no network. |

Dependencies run one way: `cli → engine → {targets, layer, config, lock, render} →
{manifest, fence, repo}`. `checks` and `brief` sit beside `cli` and depend on `engine`.

## The two ideas that carry the design

**Ownership modes plus two recorded hashes.** Every artifact ilk writes is `managed`,
`region`, `create-only`, `append-once`, or (for targets only) `merge`. The lockfile
stores two hashes per artifact, and the distinction is load-bearing:

- `Hash` — what ilk expects to find. Anything else means somebody edited its output.
- `Delivered` — the layer's own content at the last reconciliation, which is the
  common ancestor a three-way merge needs.

They diverge whenever a file legitimately differs from what the layer produces: after
a merge, or after `--accept`. Collapsing them — which an earlier version did — makes
the next apply mistake an agreed divergence for "unchanged since ilk wrote it" and
overwrite it, silently destroying the merge it had just performed. `reconcileArtifact`
in `internal/engine/reconcile.go` is the single place that decision is made.

The content behind both hashes lives in `.ilk/base/`, committed rather than ignored:
a teammate who pulls the repository needs the same ancestor, or their upgrade degrades
to a refusal for no reason they can see.

**The CLI is the interface; agent config is a projection.** Layers declare instructions,
skills, hooks and commands in a neutral form. Targets turn those into `AGENTS.md`,
`.claude/`, `.cursor/` and git hooks. Because every generated hook says only
`ilk hook run <event>`, adding a hook to a layer never rewrites an agent's config, and
adding an agent never touches a layer.

## Deliberate constraints

- **Project-scoped and monorepo-only.** One repository, one root, one `.ilk/`. No
  org-level configuration and no multi-repo resolution.
- **`.ilk/` holds everything ilk owns.** Config, lockfile, cache, and layer-shipped
  scripts. The project record lives at the repository root because it is for humans and
  agents to read, not for ilk to manage.
- **A layer governs what happens next, not what came before.** Files already present
  in a directory when its layer arrives are recorded as a baseline and exempted from
  that layer's checks. Adoption into a repository with history has to be survivable,
  and the exemption is visible and shrinkable rather than silent.
- **Declarative-first layers.** A layer that only renders files needs no consent to
  adopt, because `ilk plan` shows every byte first. Executable content from a
  non-built-in source requires `--allow-exec`.
- **Command execution is `sh -c`.** Windows is not supported for layer commands and
  hooks. The binary itself builds anywhere.

## Testing

`internal/engine/engine_test.go` holds the contract: adopt, edit around, upgrade and
drop must leave a repository exactly as it was, apart from files ilk deliberately
seeded and handed over. `internal/merge/merge_test.go` holds the other half — that a
clean merge preserves both sides' edits and an ambiguous one is reported rather than
resolved. `ilk layer test` applies the same standard to any layer,
including the built-in ones, and CI runs it against both on every push.
