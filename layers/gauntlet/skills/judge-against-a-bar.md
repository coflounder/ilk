# Judge against a bar

Compare finished work with something inspectable, and record what you found.

## The rule this exists for

The builder does not grade its own homework. A session that made a hundred small
choices has a reason for every one of them, and those reasons are exactly what makes
its own output look inevitable. It is not dishonesty; it is the ordinary effect of
having been there.

So the judging happens in a **fresh session** that is given two things:

1. **The bar** — what the work was supposed to match.
2. **The artifact** — the diff, the running thing, the screenshot, the output.

And is given **none of the builder's reasoning**. Not the plan's rationale section, not
the commit messages explaining why an approach was abandoned, not "I chose X because
Y". A critic that reads the justification before the artifact is reviewing the
justification.

## What counts as a bar

A bar is inspectable: a second person can open it and disagree with your reading of it.

| Kind of work | A bar | Not a bar |
|---|---|---|
| A feature | the acceptance criteria, one at a time | "the spec" |
| A bug fix | the failing case, and it now passes | "it works now" |
| Interface work | the wireframe or the before-screenshot, side by side with the after | "it looks fine" |
| Performance | the budget, and the number measured | "it feels faster" |
| A refactor | the suite that passed before passing after, and what it covers | "nothing broke" |
| Writing | the outline agreed, and the facts it rests on | "it reads well" |

If you cannot name one, that is the finding. Say so in the verdict and stop — work
whose bar nobody can state was never going to be judgeable, and inventing one after the
fact just judges the work against itself.

## Running it

There is no command here on purpose. What a critic is differs per repository: a
colleague, a fresh agent session, a review tool you already run. Whatever it is, three
things have to hold:

- **Separate context.** A new session, a different person, a tool that reads only the
  artifact. Not the same conversation with an instruction to be sceptical.
- **The critic reads the artifact.** Not a summary of it, and not the plan document's
  description of what was built. Open the diff. Run the thing.
- **The critic writes the verdict**, and its name goes in `critic:`.

Where the work has several independently judgeable parts, judge them separately and
recycle only the parts that failed. One verdict over a large change collapses to an
average, and an average hides the one part that is wrong.

## Writing it down

In the plan document, before setting its status:

```markdown
---
status: done
critic: fresh session, opus, 2026-08-08
---

## Bar

- The three acceptance criteria above, each read against the diff.
- `docs/evidence/checkout-wireframe.png` — what the step was agreed to look like.

## Verdict

- Opened the diff in `src/checkout/` and ran the flow against a test card.
- All three criteria hold: the total updates on quantity change, the button disables
  while the charge is in flight, and a declined card leaves the basket intact.
- The wireframe puts the total above the button and the build puts it below. The
  criteria do not mention position, so this is a difference, not a failure.

## Largest gap

- A declined card shows a generic banner, where the wireframe shows the message inline
  under the field. Not blocking — no criterion covers it — but it is the state a user
  is most likely to meet and least likely to understand.
```

Each section is read as a list: the checks count bullets, so write them as bullets.

### `## Largest gap` is not optional

Passing work has one. It is the gap that was not worth fixing, and saying why is what
distinguishes a judgement from a signature.

A critic that writes "no significant gaps" has, almost always, not looked at the
artifact. When you genuinely cannot find one, say what you would have had to see to
find one — that is falsifiable, and "looks good" is not.

## What a verdict cannot do

It cannot make failing work pass. The verdict is one check among the gates, so a repo
with a red suite is red whatever the critic concluded — a model's judgement is not an
objective gate and must never be treated as one. Its whole value is the other
direction: finding what a passing suite does not cover.

## When it fails

Recycle the specific part, with the evidence of the gap, and judge it again. Two rounds
of the same failure is a signal to escalate rather than to try a third time —
`ilk ask-human open "..."` if that layer is present.
