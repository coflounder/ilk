# Sketch before building

Draw what an interface will look like, in a form that provokes disagreement rather than
approval.

## Why this is worth an afternoon

The disagreement is coming either way. The only question is whether it arrives while the
thing is a file you can delete, or after it is a screen somebody built, tested and
reviewed. A wireframe is not a design step; it is a way of moving the argument earlier,
when it is cheap.

That only works if the sketch is genuinely cheap to throw away. The moment a wireframe
becomes expensive — a component library, a token set, three hours of styling — people
defend it instead of arguing with it, and it has become the thing it was meant to avoid.

## Fidelity is the message

| Draw | Do not draw |
|---|---|
| Where things sit, and how big they are relative to each other | Colours, shadows, real typography |
| What is on the screen and what is not | Final copy — `Lorem` or a plausible placeholder is fine |
| The order somebody moves through | Animation, hover states, transitions |
| The empty state, the error state, the loading state | Anything that requires JavaScript to see |

A sketch that looks finished gets reactions about the shade of a button. One that looks
like a sketch gets reactions about the flow. The scaffold's greyscale boxes are doing
that job, so resist tidying them up.

## How

```sh
ilk html-wireframe new "Checkout — payment step"
```

One file per screen or state. Open it in a browser, edit the HTML, reload. It is one
self-contained file on purpose: no build, no network, no dependency that can rot, so
whoever inherits this repository in two years can still open what was agreed.

Then link it from the spec:

```markdown
## Wireframe

- [Checkout — payment step](../wireframes/WF-checkout-payment-step.html)
- [Checkout — declined card](../wireframes/WF-checkout-declined-card.html)
```

### Draw the states nobody asks for

The main state is the one everybody imagines the same way, so it is the one you learn
least from drawing. The disagreements live in:

- **Empty** — what is here before there is any data, on the first day.
- **Error** — what the user sees, and *where the message sits*. This is where wireframes
  and builds diverge most often.
- **Too much** — forty rows, a name that is ninety characters, a total that does not fit.

## Ask a question

A wireframe posted with "here's the wireframe" gets "looks good", and settles nothing.
Say what you are unsure about and what you want argued with:

> This puts the total below the button because the button is what people are looking
> for. It means the number moves when the basket changes and the eye has to come back
> for it. Is that the wrong trade?

If two layouts are genuinely in contention, draw both and offer them as a choice — with
what picking each would mean. Two sketches side by side is the cheapest decision anybody
will make all week, and if the `ask-human` layer is present, this is exactly what a
`kind: decision` question is for.

## When not to

- **The change is invisible.** If nothing anybody looks at moves, `ui: true` is wrong,
  and taking it off is the fix rather than drawing a wireframe of nothing.
- **The pattern already exists.** A fourth table that behaves like the other three does
  not need a sketch; it needs a link to one of them.
- **It genuinely needs behaviour to make its point.** A drag interaction, a
  spatial-audio control, anything where the question *is* the motion. Build the smallest
  real thing and use `visual-qa` on it — but say that is what you are doing, because it
  is the expensive path and it should be a decision rather than a drift.

## Afterwards

The wireframe stays. It is what the work was agreed to be, and `visual-qa`'s screenshots
are what it turned out to be — having both is what makes comparing them somebody's job
rather than nobody's.

When the built thing deliberately differs, say so in the spec. A difference nobody
mentions reads as a mistake to the next person, and they will "fix" it.
