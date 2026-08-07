# Capture UI evidence

Show what an interface change looks like, rather than asserting that it works.

## When

A spec touches the interface; you are about to mark interface work done; or a reviewer
cannot tell from the diff what the user will see.

## Why this is not optional

You can make an interface change, run the tests, and be completely wrong: text
overlapping, a control off-screen, a modal behind its backdrop, a layout that collapses
below 400px. No test suite in the project catches any of that, and the reviewer cannot
see it without checking out the branch and running the app — which, realistically, they
will not do.

A screenshot costs a minute and moves the failure from "found in production" to "found
in review".

## Procedure

1. **Set `ui: true`** in the spec's frontmatter. That is what makes `ilk check` insist
   on evidence.

2. **Work out which states matter.** Read the acceptance criteria and list every one
   that describes something a person would *see*. Then add the states that are always
   forgotten: empty, loading, error, permission denied, and the narrowest supported
   viewport.

3. **Get the app running.** Use whatever the project already has — its dev server, its
   Playwright or Cypress setup, its Storybook. If there is no way to drive the UI
   headlessly, that is worth saying out loud: it makes every future interface change
   unverifiable, and it is a bigger problem than the change you are making.

4. **Capture each state** to `evidence/<spec-id>/<state>.png`. Name the state by what
   the reader is looking at — `empty-cart.png`, not `Checkout.test.png`.

5. **Link them from an `## Evidence` section** in the spec, one line per screenshot,
   saying which criterion it settles:

   ```markdown
   ## Evidence

   - Empty cart shows the "browse products" prompt — ![](../evidence/spec-checkout/empty-cart.png)
   - Card declined shows the retry affordance — ![](../evidence/spec-checkout/declined.png)
   - Usable at 320px — ![](../evidence/spec-checkout/narrow.png)
   ```

6. **Look at them.** Actually open them. Capturing a screenshot of a broken layout and
   linking it proudly is a real failure mode, and it is worse than no screenshot because
   it reads as verification.

## What counts as evidence

| Not evidence | Evidence |
|---|---|
| "The component renders" | A screenshot of the state a user will be in |
| The happy path only | The empty, error and narrow states too |
| A screenshot of the whole page at 1920px | The region the change affects, at a width somebody uses |
| A screenshot you did not look at | One you looked at and can describe |

## What not to do

- Do not capture only the state that was easiest to reach. That is the state least
  likely to be broken.
- Do not link a screenshot from a previous run because "nothing visual changed". If
  nothing visual changed, the spec should not be `ui: true`.
- Do not commit screenshots at 4K. They are for reading in a diff view.
- Do not treat this as a substitute for testing behaviour. A screenshot shows what it
  looks like, not what it does; both matter, and they catch different failures.
