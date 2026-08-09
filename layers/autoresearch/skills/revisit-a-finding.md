# Revisit a finding

Decide what to do with a finding that has run out of date, or that reality has just
contradicted.

## When

`ilk check` reports `research.fresh`; `ilk autoresearch due` lists something; or you
have just watched the world disagree with a document in
`{{ index .Caps "research.findings" }}/`.

## The one rule

**Renewing `expires:` is a claim that you opened a source again.** It is not a
formality, and it is the only place in this layer where dishonesty is both easy and
invisible. A date bumped without re-reading produces a document that looks freshly
verified and is not, which is worse than a document that is visibly overdue.

If you are not going to re-read the sources, do not touch the date. Leave the check
failing — an overdue finding is legible, and a falsely renewed one is not.

## Procedure

1. **Read the `## What would change this` section first.** It was written to tell you
   exactly what to look at, which is usually far less work than the original search.
2. **Open the `sources:`.** Every one. A dead link is itself a finding: the vendor
   reorganised, and whatever you concluded may have gone with it.
3. Then take one of four routes.

### Still true

Move `expires:` and `updated:` forward and change nothing else. If you re-read the
sources and they still say what the document says, that is the whole answer.

Choose the new date from what you just saw, not from the old interval. A page that
now carries a "changes in 2027" banner expires in 2027, whatever the habit was.

### True, but narrower than it says

The most common outcome, and the one most often missed. The finding was right for the
plan, region, version or account type you tested and is now being cited generally.
Add the qualifier to `## Finding`, drop `confidence:` a step, and say in
`## What would change this` which other case is unverified.

### Wrong

Correct it in place, in the document that was wrong. Do not delete it and write a
fresh one.

Say three things: what the document claimed, what is actually true, and — if you can
tell — which of the two was ever true. "Stripe raised this from 25 to 100 in March"
and "this was never 25; the 25 was the test-mode limit" call for completely different
levels of trust in everything else here, and only the document itself can record the
difference.

Then set `confidence:` from the corrected evidence, not from embarrassment. A
correction that lands at `low` because you could only find one source is honest;
one that lands at `high` because you just checked carefully this time is the same
mistake again.

If the repository has the `archive` layer and the finding is not merely wrong but no
longer a question anybody has, `ilk archive it <file>` keeps the reasoning without
leaving it where it will be cited.

### No longer a question

The vendor is gone, the library was dropped, the decision it informed was reversed.
Archive it, or delete it and note the removal in the log. What you must not do is
leave it with a renewed date because renewing was quicker than deciding.

## What a wrong finding costs, and why that is the design

This layer is one confident wrong answer away from being worse than nothing, because
a written finding is read *instead of* checking. Three things hold that risk down,
and all three are only as good as the sessions that maintain them:

- `confidence:` makes a shaky finding legible as shaky, so a reader can decide
  whether to rely on it or verify it.
- `## What would change this` makes re-checking cheap enough to actually happen.
- `expires:` forces the question to be asked again on a schedule the world sets,
  not on one this repository can detect.

A correction is therefore not a failure of the layer — it is the layer working.
The failure mode is a quiet edit, or a bumped date, that leaves no trace of having
been wrong.

## What not to do

- Do not bump every overdue date in one pass to get `ilk check` green. Do them one at
  a time, or leave them failing and say why in the commit.
- Do not lower `expires:` to a very near date to defer the decision. That is the same
  avoidance with more churn.
- Do not raise `confidence:` while correcting a finding unless the new sources are
  genuinely better than the old ones.
- Do not rewrite history so the document reads as though it was always right.
