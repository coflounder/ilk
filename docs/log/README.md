---
date: 2026-01-01
title: What belongs in docs/log/
---

# docs/log/ — what happened

Dated entries, append-only in spirit. This is the directory that lets a session
started today understand a decision made six weeks ago without asking anyone.

Write an entry when something happened that a future reader would otherwise have to
reconstruct: a release, an incident, a migration, a plan abandoned and why, a
surprising discovery about how the system actually behaves.

Do not write an entry for every commit. Git already has those, and a log nobody
reads is worse than no log.

## Filename grammar

`YYYY-MM-DD-kebab-slug.md`. `2026-08-06-shipped-billing.md`.

## Frontmatter

```yaml
---
date: 2026-08-06
title: Shipped the billing rewrite
---
```

```
ilk record log "Shipped the billing rewrite"
```

An entry that changes what is *true* should also update `../reference/`.
The log records that something happened; the docs record what is now the case. Both
sides have to balance.
