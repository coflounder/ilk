---
id: arch-system-overview
title: System overview
status: current
updated: 2026-08-07
covers:
  - internal/**
  - cmd/**
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
  engine.Load             resolve every layer in the config from its source
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

`plan` and `apply` are separate because adding a layer should never be a surprise.
Nothing is written until the whole blast radius has been shown.

## Packages

| Package | Responsibility |
|---|---|
| `internal/fence` | The ownership markers. Upsert, extract and remove a block inside a file somebody else owns, losslessly. |
| `internal/manifest` | The layer schema and its validation. |
| `internal/layer` | Resolving a layer reference to a manifest plus a file tree: built-in, local path, or git. |
| `internal/config` | `.ilk/config.yaml` — desired state, comment-preserving on save. |
| `internal/lock` | `.ilk/lock.json` — provenance hashes for everything ilk wrote, keyed by *owner*: a layer, `ilk/core`, or a `target:*` adapter. |
| `internal/basestore` | `.ilk/base/` — content-addressed copies of what ilk last wrote, so upgrades can merge rather than refuse. |
| `internal/merge` | Line-based three-way merge. Conservative by construction: merges disjoint or identical changes, refuses anything else. |
| `internal/render` | Templating. Deliberately small; layers should be readable by whoever is deciding to add them. |
| `internal/targets` | Agent adapters. Project neutral artifacts into what each agent reads. |
| `internal/engine` | Desired-state computation, planning, applying. |
| `internal/checks` | The validator runner and the built-in checks. |
| `internal/mirror` | Reconciling record documents with an external tracker. Owns identity, diffing and refusal; knows no provider. |
| `internal/contrib` | Turning what a repository learned about a layer into a proposal its maintainer can judge. |
| `internal/brief` | The session-start packet. |
| `internal/cli` | Command surface. |
| `internal/builtin` | Layers embedded in the binary, so `ilk init` needs no network. |
| `internal/registry` | The layer index, embedded so `ilk search` works offline. |

Dependencies run one way: `cli → engine → {targets, layer, config, lock, render} →
{manifest, fence, repo}`. `checks` and `brief` sit beside `cli` and depend on `engine`.

## The ideas that carry the design

**Ownership modes plus two recorded hashes.** Every artifact ilk writes is `managed`,
`region`, `create-only`, `append-once`, or — for targets only — `merge` and `symlink`.
The two target-only modes exist for the same reason: which file an agent reads, and
whether it can be a link, is the adapter's knowledge rather than the layer's. A symlink's
content *is* its target, so it is planned, applied and pruned without ever being read
through. The lockfile stores two hashes per artifact, and the distinction is
load-bearing:

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

**Writing to somebody else's system reuses the same discipline.** `ilk mirror`
reconciles record documents with a tracker, and the shape is `plan` then `apply`
again: nothing is written until the whole plan has been seen. Core owns identity (a
frontmatter key it owns on each document), diffing, and the refusals — a document whose
title could mean two tracker items is named rather than guessed at, because a wrong link
is silent and every later sync compounds it. A layer supplies three commands that know
the provider and normalise it to `{id, title, status, url}`, which is why core has never
learned what a GitHub Project is. Nothing is ever deleted remotely; an item nobody claims
is reported.

**Improvement travels in both directions.** `ilk upgrade` carries a layer's changes
down into repositories that have tuned it, three-way merging rather than overwriting.
`ilk contribute` carries what those repositories learned back up. The second is only
possible because the first already required knowing exactly what the layer delivered:
the lockfile's `Delivered` hash and the base store behind it are the same machinery,
read in the other direction. What ilk gathers is evidence — the divergence, how long
it held, defaults overridden, checks that could not run — and never the argument.
Whether a local edit is a fix everybody needs or a quirk of one repository is a
judgement, and a generated paragraph guessing at it would read like an argument and
carry none.

## Deliberate constraints

- **Project-scoped and monorepo-only.** One repository, one root, one `.ilk/`. No
  org-level configuration and no multi-repo resolution.
- **Directories are grouped; ordering is presentation.** A layer places a directory with
  `group:` and `name:`, and the group's `README.md` carries a generated index ordered by
  each layer's declared `order:`. No ordinal ever reaches a path — see
  [DEC-groups-not-ordinals](DEC-groups-not-ordinals.md). Groups are what keep a
  repository's root from growing one folder per layer.
- **A capability may carry a value, and a path is the reason.** `provides:` accepts
  `record.plans: "{{ .Vars.group }}/{{ .Vars.plans_dir }}"`, so a consumer reads
  `{{ index .Caps "record.plans" }}` rather than redeclaring where the plans are. A value
  renders against `.Vars` and `.Repo` only: it cannot read `.Caps`, because that is what
  is being resolved. Layers depend on capabilities, never on each other, and this is what
  lets that hold for places as well as commands.
- **`.ilk/` holds everything ilk owns.** Config, lockfile, cache, and layer-shipped
  scripts. The project record lives at the repository root because it is for humans and
  agents to read, not for ilk to manage.
- **Staleness is coupling, not decay.** A document declares `covers:` and goes stale
  when git says those paths moved, measured in commits rather than days. No universal
  expiry can suit projects of different maturity, and a timer trains people to bump a
  date rather than read a document. The measurement uses a commit range rather than a
  timestamp, because several commits can share a second.
- **A layer governs what happens next, not what came before.** Files already present
  in a directory when its layer arrives are recorded as a baseline and exempted from
  that layer's checks. Landing in a repository with history has to be survivable,
  and the exemption is visible and shrinkable rather than silent.
- **Declarative-first layers.** A layer that only renders files needs no consent to
  add, because `ilk plan` shows every byte first. Executable content from a
  non-built-in source requires `--allow-exec`.
- **Command execution is `sh -c`.** Windows is not supported for layer commands and
  hooks. The binary itself builds anywhere.
- **A proposal refuses rather than being edited.** `ilk contribute` blocks on a
  credential and on an unwritten case, and strips nothing. Editing evidence on the way
  out would change what upstream is being asked to judge, and quietly sending
  something other than what the repository found is worse than sending nothing.
- **The record is the source of truth; a mirror is one-way.** `ilk mirror` makes the
  tracker match the markdown and never the reverse, so "which one is right" is never a
  question. A tracker that has drifted is a change to push, not an update to pull.

## Testing

`internal/engine/engine_test.go` holds the contract: add, edit around, upgrade and
remove must leave a repository exactly as it was, apart from files ilk deliberately
seeded and handed over. Its `snapshot` helper records a symlink by where it points rather
than following it, so a link ilk failed to clean up cannot pass as an ordinary absence.
`internal/lock/lock_test.go` holds a smaller but sharper one: a lockfile written by an
earlier version must still load. It exists because renaming a field in that file made
every existing lockfile read as empty — ilk concluding it had written nothing, and
therefore having nothing to clean up or protect — with the whole suite green, because
nothing until then had read a file the current code did not write. `internal/merge/merge_test.go` holds the other half — that a
clean merge preserves both sides' edits and an ambiguous one is reported rather than
resolved. `internal/mirror/mirror_test.go` holds the third — a fake provider stands in
for a tracker, so identity, diffing and every refusal are verified with no network and no
account, which is also the only way they can be verified in CI. `ilk layer test` applies
the same standard to any layer, including the built-in ones, and CI runs it against both
on every push.
