---
id: docs-readme
title: What belongs in docs/
status: current
updated: 2026-01-01
---

# docs/ — what is true now

Present tense, current state. If a statement in here is no longer true of the code,
it is a bug in this directory.

This is not a place for history (that is `log/`) or for intent
(that is `plans/`). "We are going to move to Postgres" does not belong
here. "The system uses Postgres" does, once it does.

## Filename grammar

`TYPE-kebab-slug.md`, where `TYPE` is uppercase. `ARCH-system-overview.md` is a
record; `arch_system_overview.md` is an error `ilk check` will catch and tell you
how to fix.

Common types — extend the set freely, the check only requires that the shape holds:

| Type | For |
|---|---|
| `ARCH` | Architecture, boundaries, data flow |
| `API` | Interfaces and contracts |
| `OPS` | Deployment, runbooks, on-call |
| `DEC` | A decision and the reasoning behind it |
| `REF` | Reference material, conventions, glossaries |

## Frontmatter

Every document carries:

```yaml
---
id: arch-system-overview     # stable; other documents link by it
title: System overview
status: current              # current | draft | superseded
updated: 2026-08-06          # bump when you have re-read this against the code
---
```

`ilk check` flags documents here that have not been touched since the code they
describe moved on. `updated:` is your acknowledgement that you have looked.

Create one with the naming and frontmatter already correct:

```
ilk record new docs "System overview"
```
