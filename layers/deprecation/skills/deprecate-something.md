# Deprecate something

Put a removal date on code that is going away, in a form the repository will hold you
to.

## When

You are replacing an interface, retiring a feature flag, moving an endpoint, or
dropping support for something — and the old thing has to stay for a while because
callers still use it.

The other trigger is the important one: **you are about to write `// remove after v3`
in a comment.** Nothing can act on that comment. No check reads it, no brief mentions
it, and the version it names will ship without anybody noticing. Write the document.

## Write the notice

```
ilk record new docs "legacy auth"
```

Then rename it to `{{ .Vars.prefix }}legacy-auth.md` and give it this frontmatter:

```yaml
---
id: dep-legacy-auth
title: Legacy auth is being removed
status: current
updated: 2026-08-07
announced: 2026-08-07
remove_after: 2026-11-01
covers:
  - src/auth/legacy/**
  - src/middleware/legacy-session.go
---
```

Three fields carry the whole mechanic:

- **`covers:`** is what the deprecation is *about*. It is matched against what git
  tracks, so the repository — not your memory — decides whether the code is still
  there. Name the paths that must be empty for this to be finished, and nothing else:
  a pattern that is too wide keeps the notice alive after the real work is done.
- **`remove_after:`** is the date after which those paths must be gone. Past it,
  `deprecation.overdue` fails.
- **`announced:`** is the day you wrote this, and it never changes. Its only job is to
  make an extension visible: if `remove_after` moves and `announced` does not, the
  growing gap between them is right there in the diff.

## Then say what a caller has to do

The body is for the reader who just hit the deprecation. At minimum:

- **What replaces it**, with a link to the current document or the new symbol.
- **How to migrate**, concretely enough to follow — the before and the after.
- **Why the date is that date.** "The next release train" and "when the last caller
  is off it" are real reasons. "Three months" is a reason nobody will defend later,
  and a date nobody will defend is a date that gets extended.

## Pick a date somebody has agreed to

This is the part that decides whether the deprecation ever completes. A date chosen
because it felt about right will arrive as an interruption, and an interruption gets
exempted. A date tied to something the team already committed to — a release, a
migration, a customer contract ending — arrives as a thing that was always going to
happen.

If nobody will commit to a date, that is worth knowing now: what you have is a wish
that the code goes away, and writing it down as a deprecation only moves the argument
to the day the check fires.

## Announce it where the work happens

The notice does its job through `ilk check`, which means it reaches whoever runs
checks. If the callers are in another repository or another team, the document is not
the announcement — send the announcement too, and link to the document.
