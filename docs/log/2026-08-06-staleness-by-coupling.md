---
date: 2026-08-06
title: Staleness measured by coupling rather than decay
---

# Staleness measured by coupling rather than decay

A stale document is as dangerous as a bloated one. An agent reads it confidently and
acts on something that stopped being true months ago, and unlike slop it leaves no
trace of having misled anyone.

## What was wrong

`record.stale` compared each document's `updated:` field against
`LastCommitTime(".")` — the most recent commit **anywhere in the repository**. In any
active project that is roughly now, so the check collapsed into a calendar timer:
"you have not touched this document in 45 days." It measured attention, not accuracy.

That is also the version of the idea that cannot work. A universal expiry is wrong for
almost everybody: a week is an eternity for a prototype and nothing at all for a
subsystem that has been stable for two years. And a timer trains exactly the wrong
habit, because the cheapest way to satisfy it is to bump the date without reading
anything.

## What replaced it

Documents declare what they describe:

```yaml
covers:
  - src/payments/**
  - migrations/*payment*
```

Staleness is then the number of commits touching those paths since the document was
last read. This calibrates itself without being told: code nobody has touched cannot
make its documentation stale however old it is, and code rewritten yesterday makes it
stale immediately. The project's own pace sets the pace of review.

`ilk record review <file>` prints those commits and the diffstat, then records the
review — so acknowledgement is informed rather than a date somebody typed.

Two silent failures are now loud. A document with no `covers:` is never stale, which
is indistinguishable from one that is always right. A `covers:` pattern matching
nothing is worse, because it looks like it is working; a single typo would exempt a
document for ever.

## Two things the tests forced

**Timestamps are not precise enough.** Commits made in the same second as the
document's own commit were counted against it, because `git log --since` is inclusive.
Measuring in a commit range (`<doc commit>..HEAD`) is exact and has no clock in it at
all. The `updated:` field still wins when it is newer, which covers the case of having
just run a review and not committed yet.

**The absolute backstop needs a different signal.** `max_age_days` — off by default,
for documents that go stale because the world moved rather than the code — trusts only
the declared `updated:`. A commit touching a document proves somebody changed it, not
that anybody read it, and this is the one check asking the second question.

## An edge worth recording

Not every document describes code. This repository's own design proposal is reasoning,
not description; forcing a `covers:` on it would have been a lie. `covers: []` says so
explicitly, and is accepted where a missing key is not — the difference being that one
is a decision a reader can see and the other is an oversight.
