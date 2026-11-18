# Proposals

What repositories using these layers learned, sent back.

Each file is one repository's experience of one layer: what it changed after the
layer wrote it, what the layer asked for that it could not give, and — the part no
tool produces — why that is likely to be true elsewhere too.

## The life of a proposal

| `status:` | meaning |
|---|---|
| `open` | waiting for a decision |
| `reviewed` | decided, with `verdict:` and a `## Verdict` section giving reasons |

`verdict:` is `accepted`, `declined` or `needs-work`.

`ilk check` enforces the parts that can be enforced: a proposal names its layer and
its origin, a reviewed one records who decided and why, unwritten contributor
sections fail rather than being reviewed, and the open queue stays short enough to
be a promise.

## What review is for

Not gatekeeping. A proposal is a repository telling you something about your layer
that you could not have found out yourself, and the failure mode is not accepting a
bad change — it is the contributor concluding that sending things here is not worth
the effort.

So: decide, and say why. A declined proposal with a reason is a good outcome. An
open proposal nobody has looked at in a month is the only bad one.

The `review-a-proposal` skill has the rubric.

## Accepting one

Change the layer, bump its version, and say in the verdict which version carries it.
Adopters get it through `ilk upgrade`, which three-way merges it into repositories
that had tuned the layer — including the repository whose tuning prompted the
proposal in the first place. That is the loop closing: their local fix stops being
local, and stops being theirs to maintain.
