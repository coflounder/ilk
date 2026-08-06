---
date: 2026-08-06
title: A toolkit layer, and the first community layers
---

# A toolkit layer, and the first community layers

Two additions, aimed at the same gap: ilk was usable by somebody who had read its
source, and not much use to anybody else.

## `ilk/toolkit`

Five skills covering the operations an adopter actually needs: adopting a layer,
applying changes, resolving a conflict, configuring capabilities and targets, and
writing a layer. Now in both `init` profiles, so a base `ilk init` leaves an agent
able to operate the tool rather than guessing at it.

It ships no `instructions:` at all. Skills cost nothing until their situation applies,
so the layer costs a handful of index lines in `AGENTS.md` and nothing else — a tool
that explains itself in every context window would have misunderstood its own budget
advice.

The conflict skill was the one worth writing carefully. `--merge-markers`, `--accept`
and `--force` are three answers to the same prompt with very different consequences,
and the failure mode is somebody reaching for `--force` because it sounds decisive.

## The first community layers

Three, in `layers/`, each taking one MetaHarness principle and reducing it to
something that works on plain markdown and git:

- **`blueprint`** — every spec belongs to an epic and a milestone that exist, and
  carries acceptance criteria. The essay's "a spec with tickets and no checkpoints is
  an error", without needing a ticket system.
- **`compound-lessons`** — every lesson names the durable change it produced. The
  check is on a frontmatter field, which sounds trivial and is the entire point: "we
  will be more careful" cannot pass as an outcome.
- **`archive`** — supersede rather than delete, and nothing live may cite the archive.

Building them needed three new generic checks rather than three bespoke ones:
`builtin.references` (a frontmatter field resolving to another document's id),
`builtin.section` (a required heading with content under it), and a `forbid_prefix`
argument on `builtin.links`. Each is reusable by layers nobody has written yet, which
is the test of whether a check belongs in the core.

`builtin.frontmatter` also gained `match:`, after the lessons layer quietly demanded
`encoded_as` on every document in `docs/` rather than only on lessons. Caught by
adopting the layer and reading the output, not by the manifest validator.

## Discovery

`ilk search` reads an index embedded in the binary, so it works offline. The index is
a list of names and sources — no server, no account, and publishing is a pull request
against one file. CI verifies every indexed entry actually resolves, because an index
advertising layers nobody can adopt is worse than no index.

The first version of that CI check passed while validating only four of six entries:
`ilk search` hides already-adopted layers by default, and the check had not asked for
`--all`. A check that quietly examines less than it claims is the kind this repository
is supposed to be good at catching.
