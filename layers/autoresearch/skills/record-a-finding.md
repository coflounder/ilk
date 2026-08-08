# Record a finding

Turn something you learned about the world outside this repository into a document
the next session can use without paying for it again.

## When

You have just read vendor documentation, tested an external API, compared libraries,
or established why something does not work. Also when `ilk check` reports a finding
with no source or no falsification clause.

The test for whether it belongs here: **could this be wrong tomorrow because somebody
outside this project changed something?** If yes, it is research. If it can only be
wrong because *we* changed something, it is architecture — write it in
`{{ .Vars.group }}/reference/` where staleness is measured against the code.

## Write it

```
{{ index .Caps "research.findings" }}/RES-stripe-rate-limits.md
```

One question per file. If you find yourself writing "and also", that is a second
finding, and merging them means they expire together for no reason.

```yaml
---
id: res-stripe-rate-limits
title: Stripe's real rate limit on the Payment Intents API
status: current
question: How many Payment Intents requests per second before 429s, and what happens at the limit?
sources:
  - https://docs.stripe.com/rate-limits
  - https://github.com/stripe/stripe-node/issues/1234
confidence: high
expires: 2026-11-05
updated: 2026-08-07
---
```

Then two sections:

```markdown
## Finding

100 requests/second in live mode, 25 in test mode, per account rather than per key.
Over the limit returns 429 with `Retry-After` in seconds; the official SDKs do not
retry automatically.

## What would change this

- A different number on https://docs.stripe.com/rate-limits.
- A 429 observed below 100 rps in production.
- Stripe moving Payment Intents to a new API version.
```

## The three fields people get wrong

**`sources:` are pages you opened, not pages that probably exist.** A URL you inferred
from a naming pattern is a fabrication with a citation attached, which is strictly
worse than no citation. If the answer came from running something rather than reading
something, cite the command and its output in `## Finding` and put the vendor's page
in `sources:` only if you read it.

**`confidence:` is about provenance, not about how sure you feel.** `high` means the
vendor said it or you observed it. `medium` means one source, or agreement among
sources that are all downstream of each other. `low` means you inferred it, or the
sources disagree — and a `low` finding must say what it would take to raise it.

**`expires:` comes from how fast the subject moves.** 90 days for pricing, quotas and
limits; 180 for a library API inside a pinned major; 30 for anything in beta; the
announced date for a deprecation. Picking a year because nothing feels urgent is how
this directory becomes a museum.

## What would change this

The section is required because it is the cheapest available defence against the one
way this layer makes things worse. A written finding carries more authority than a
fresh guess — a later session will read it instead of checking, which is the point
and also the risk. Naming the observation that would overturn it does two things: it
forces you to notice whether the finding is falsifiable at all, and it tells the next
reader exactly what to look at rather than making them redo the whole search.

If you cannot write a single line for that section, you have not established a
finding. You have an opinion, and it belongs in the prose of a decision document
where nobody will mistake it for a checked fact.

## Before you call it recorded

```
ilk check --only research.frontmatter,research.sources,research.falsifiable,research.fresh
```

Then read the `## Finding` section as somebody who was not in the session. Most
findings that turn out to be useless are perfectly clear to the agent that wrote
them and ambiguous about which version, which region or which plan they applied to.

## What not to do

- Do not record what the repository already shows. "We use Postgres 16" is
  architecture; "Postgres 16 changed the default for `X`" is research.
- Do not write a finding from a single search-result snippet. Open the page.
- Do not paste a long extract from a vendor's documentation. Link it and state the
  conclusion — a copy goes stale silently and nothing here will notice.
- Do not use `confidence: high` to make a check quieter. Nothing checks the value,
  which is exactly why overstating it is corrosive rather than merely wrong.
