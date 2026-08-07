---
id: dec-fenced-regions-over-whole-files
title: ilk owns fenced regions, not whole files
status: current
updated: 2026-08-06
covers:
  - internal/fence/**
  - internal/engine/plan.go
---

# ilk owns fenced regions, not whole files

## Context

ilk has to put content into files that already exist and belong to somebody else —
`AGENTS.md`, `.gitignore`, CI workflows, an agent's settings file. Several layers may
want to write into the same file, and a human will certainly want to write in it too.

## Decision

ilk owns **fenced regions**, marked with `ilk:begin` / `ilk:end` comment lines carrying
the owning layer and region name. Files ilk fully owns are the exception, reserved for
artifacts that exist only because a layer does.

## Alternatives considered

**Own the whole file.** Simplest to implement, and what most generators do. Rejected
because it makes `AGENTS.md` unusable: the file where a human most wants to write
project-specific guidance would be the one file they cannot touch. It also makes two
layers contributing instructions impossible without one clobbering the other.

**Separate include files per layer, referenced from a manifest.** Clean ownership, no
markers. Rejected because no coding agent reads includes — they read one file. The
indirection would exist only for ilk's convenience and would cost the thing that
matters.

**Own the file, but merge structurally on each write.** Works for JSON, fails for
markdown, which is most of what ilk writes. Retained only for `.claude/settings.json`,
where there is no comment syntax to fence with.

## Consequences

Makes easy: several layers contributing to one file; a human editing freely around
generated content; `ilk rm` removing exactly what a layer added; detecting an edit inside
a generated block and refusing rather than silently overwriting it.

Makes hard: the marker lines are visible and slightly ugly, and a file type with no
comment syntax needs a bespoke merge function written in Go. Region content also cannot
be reordered by a human — it lives where the marker is.

## Revisit when

A file type ilk must write into has neither comment syntax nor a structural merge —
at that point the region model has genuinely run out, rather than merely being
inconvenient.
