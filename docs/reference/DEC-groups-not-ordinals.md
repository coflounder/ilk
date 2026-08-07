---
id: dec-groups-not-ordinals
title: Directories are grouped, and ordering lives in a generated index
status: current
updated: 2026-08-07
covers:
  - internal/manifest/manifest.go
  - internal/engine/desired.go
  - internal/builtin/layers/record/layer.yaml
---

# Directories are grouped, and ordering lives in a generated index

## Context

Every layer that wanted a directory put one at the repository root. With the layers
that ship here, a repository ended up with `docs/`, `plans/`, `log/`, `scratch/`,
`questions/`, `evidence/`, `proposals/` and `.archived/` — eight top-level folders, none
of which say how they relate to each other, and one more for every layer added.

The obvious fix is named groupings with ordered children: `docs/00-reference`,
`docs/01-log`, `infra/00-dns`. We considered exactly that, including a variant where the
ordinal is assigned once when a layer arrives and pinned in `.ilk/config.yaml` so it is
never recomputed behind anyone's back.

## Decision

**Groups are structure and appear in paths. Ordering is presentation and appears only in
a generated index.**

A layer declares a directory as `group:` plus `name:`, and the resolved path is
`group/name` — `docs/plans`, `infra/dns`. It may also declare an `order:`, which decides
nothing about the path and only sorts the directory within its group's
`README.md`, a fenced region ilk owns.

Groups come from a canonical set (`docs`, `infra`, `ops`), and a layer may declare its
own. A directory that genuinely belongs somewhere specific still uses `path:`;
`scratch/` stays at the root, because filing the ungoverned annex under the record would
make it look governed.

## Why not ordinals in the path

Three reasons, in the order they became decisive.

**Layers cannot coordinate a global integer namespace.** `ilk/archive` and somebody's
`acme/runbooks` both want a slot in `docs/` and neither can see the other. Any scheme
that puts a shared integer in a path requires either central assignment or collisions,
and an ecosystem where publishing a layer means claiming a number is one nobody will
publish into.

**The number becomes load-bearing everywhere else.** Ordinals in paths are quoted by
check arguments, `covers:` globs, cross-document links and every capability value. A
renumbering stops being a rename and becomes a repository-wide rewrite, and the parts it
misses fail silently — a `covers:` pattern matching nothing looks exactly like a document
that is never stale.

**The number encodes the wrong relation.** `00-reference` before `01-log` implies
sequence, but the relation between reference and log is *category*, not order. Nothing is
read first.

A generated index is strictly better at the job the numbers were for: it carries each
directory's stated purpose alongside its name, reorders for free, and cannot drift from
the directories it lists because it is derived from them.

## Consequences

- One folder at the root per group instead of one per layer. In this repository the root
  went from nine entries to six.
- A layer can no longer assume where a peer's directory is, which forced the related
  change: `provides:` now carries a value, so `record.plans` names a path and consumers
  read `{{ index .Caps "record.plans" }}` instead of redeclaring `plans_dir: plans`.
  Six layers were doing exactly that, each with a comment saying it had to match.
- Moving a directory is now a variable change plus `ilk apply`, and the prune path
  handles it: managed files are deleted, seeded files are left behind with a note, and
  a non-empty directory is reported rather than removed.
- Existing repositories must move their own files. ilk creates the new directories and
  cleans up what it owns, but it will not move a document a human wrote — see the
  upgrade note in the log entry for 2026-08-07.
- **`ilk layer validate` cannot check a templated group reference.** `group: "{{ .Vars.group }}"`
  has no value until a repository supplies one, so those are checked at plan time
  instead, where the group is resolved and the error can name it.

## Rejected: ordinals pinned at add-time

The strongest alternative. ilk assigns `docs/20-plans` once, records it in
`.ilk/config.yaml`, and never recomputes it; `ilk layout renumber` is an explicit
command that plans the renames first.

This solves the churn objection and none of the others. Two independently published
layers still collide over slots, the numbers still propagate into globs and links, and
the config file grows a path table that has to stay in agreement with the layers. It buys
a filesystem-level ordering that a generated index provides better, at the cost of making
every path in the repository carry a number that means less than it appears to.
