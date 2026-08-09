---
id: spec-checkout-totals
title: The basket total updates as quantities change
status: done
updated: 2026-08-08
critic: fresh session, opus, 2026-08-08
---

# The basket total updates as quantities change

## Acceptance criteria

- Changing a quantity updates the total without a page load.
- The pay button is disabled while a charge is in flight.
- A declined card leaves the basket intact.

## Bar

- The three acceptance criteria above, read one at a time against the diff.
- `docs/evidence/checkout-wireframe.png` — what the step was agreed to look like.

## Verdict

- Opened the diff in `src/checkout/` and ran the flow against a test card.
- All three criteria hold.
- The wireframe puts the total above the button and the build puts it below. No
  criterion mentions position, so this is a difference rather than a failure.

## Largest gap

- A declined card shows a generic banner where the wireframe shows the message inline
  under the field. Not blocking, but it is the state a user is most likely to meet and
  least likely to understand.
