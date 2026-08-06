# Review changes

Review a diff the way this repository expects, before it is proposed to anyone else.

## When

Preparing a pull request, or reviewing someone else's change.

## Read in this order

Reviewing a diff top-to-bottom finds typos and misses design errors. Read it in
order of what is expensive to fix:

1. **Does this solve the stated problem?** Find the plan slice or reported behaviour
   it claims to address. A correct implementation of the wrong thing is the most
   expensive defect in the change, and the only one that is nearly free to catch now.
2. **What does it make hard later?** New coupling, a boundary crossed, a decision
   quietly reversed. If it contradicts a `DEC-*.md`, that is either a mistake or a
   decision worth recording — not something to leave implicit.
3. **Correctness in the changed lines.** Error paths, boundaries, concurrency, the
   case where the input is empty and the case where it is enormous.
4. **What the diff does not show.** A changed function with unchanged callers. A new
   field no migration populates. A config default that only matters in production.
   This is where diffs mislead most reliably.
5. **Tests.** Not "are there tests" — would these tests have failed before the change?
   A test that passes against both the old and new code asserts nothing.
6. **The record.** If this changes what is true, `docs/` should move in the same
   change. `ilk check` will flag it if not.

## Writing the review

- Lead with what would block a merge; everything else is commentary and should be
  labelled as such.
- Be concrete: the file, the line, the input that breaks it. "This feels fragile" is
  not actionable; "this panics when `items` is empty, see line 40" is.
- Say what is good, briefly and specifically, when it is genuinely good. A review
  that only ever finds fault trains people to discount it.
- Distinguish "this is wrong" from "I would have done it differently". Only the first
  is a review comment.

## Before proposing your own change

Run `ilk check`. Read your own diff in full. Then write the description: what changed,
why, what you ran, and what you did not verify.
