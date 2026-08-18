---
id: plan-docs-onboarding
title: Docs and onboarding
status: active
updated: 2026-08-18
covers:
  - README.md
  - docs/reference/**
---

# Docs and onboarding

The layer queue is deferred in favour of this. The reasoning: the M2 acceptance
criterion — somebody outside this repository publishes a layer, and adopting it
needs no changes to ilk — has never been exercised, and it cannot be exercised
by writing more first-party layers. It is exercised by a stranger getting from
zero to a working repository, and today every path a stranger would take runs
through documents written for people who were already here.

## Outcome

A developer who has never seen ilk installs it, runs `ilk init`, adopts a layer,
and understands what just happened — without reading source, and without asking
anyone. The measure is the first fifteen minutes, not the completeness of the
reference.

## What exists, and for whom it was written

- `README.md` — closest thing to an introduction; part pitch, part reference.
- `docs/reference/REF-design-proposal.md` — the argument, written to decide, not
  to teach.
- `docs/reference/REF-writing-layers.md` — good, but it is the second document a
  layer author needs, and the first does not exist.
- `docs/reference/ARCH-system-overview.md` — internals, for contributors.
- The `toolkit` layer's skills — onboarding for *agents*, and the strongest
  onboarding surface ilk has. Humans have no equivalent.

The gap is consistent: everything answers "how does this work?", nothing answers
"what do I do first?".

## Slices

### D0 — An install that is one line

There are no tagged releases, so today "install" means `go build`. Cut a first
tagged release with `goreleaser` or equivalent: binaries for macOS and Linux, a
`curl | sh` installer, and a Homebrew tap. This is a prerequisite for every
sentence of onboarding prose; nothing else in this plan can honestly begin
"install ilk" until it lands.

*Accepted when:* a machine with neither Go nor this repository gets a working
`ilk` binary from one copy-pasted line.

### D1 — The first fifteen minutes

A quickstart that walks one real path: install, `ilk init` in an existing
repository, read what appeared and why, `ilk add` one layer, `ilk check`, fix a
failure by following its printed fix, `ilk rm`, see the repository restored.
Every command's output shown, every created file explained in one line. Ends by
naming the three ideas that carry the tool — ownership modes, capabilities,
projection — each in a sentence, each linking to the reference.

*Accepted when:* someone unfamiliar with ilk completes it in under fifteen
minutes and can say afterwards what `ilk apply` will and will not touch.

### D2 — The layer author's path

The missing first document for authors: motivation and the mental model before
`REF-writing-layers.md` supplies the contract. `ilk layer new` → edit → 
`ilk layer test` → publish to a git repository → `ilk add gh:you/yours`, as one
narrative with a small, real example — not a toy that does nothing, but the
smallest layer that earns its keep.

*Accepted when:* the path from "I have a practice worth sharing" to "someone
else adopted it" is documented end to end, and following it exercises the M2
acceptance criterion for real.

### D3 — A docs site

Pulled forward from M4. The record stays canonical in-repo; the site is a
projection of it — quickstart, the two author paths, reference, and the layer
index. Static, generated, boring.

*Accepted when:* every document above is readable at a URL, and regenerating the
site is one command in CI.

## Boundaries

- No new layers while this plan is active; the queue in
  [PLAN-layer-queue](PLAN-layer-queue.md) resumes afterwards in its stated order.
- No marketing site. The audience is a developer deciding in one sitting whether
  ilk is worth adopting.
- The record's own documents stay written for maintainers; onboarding prose is a
  separate surface, and the two are allowed to repeat each other.

## Open questions

1. **Where does the quickstart live?** In-repo under `docs/` keeps it governed
   and staleness-checked; a site-only page reads better. Probably in-repo with
   the site as projection, consistent with everything else ilk does.
2. **Does D0 imply `ilk self update`?** A curl-installed binary has no package
   manager watching it. `ilk self` exists; whether it should learn `update`
   before the first external users arrive is a small question with a long
   support tail.
3. **Should `ilk init` end by printing the quickstart's next step?** The best
   onboarding surface is the tool's own output; the cost is coupling prose to
   the binary.
