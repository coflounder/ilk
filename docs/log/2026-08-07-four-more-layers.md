---
date: 2026-08-07
title: Four more layers, and two generic checks that made them cheap
---

# Four more layers, and two generic checks that made them cheap

Shipped `ask-human`, `dev-loops`, `plan-hygiene` and `visual-qa`, taking the collection
from three layers to seven.

## The pattern that is holding

All four needed the same two additions to the check vocabulary, and neither is specific
to any of them:

- **`builtin.limit`** — cap how many documents may be in a state. With no `max:` it
  forbids the state outright, which is how `ask-human` says an open blocking question is
  not an acceptable resting place; with a `max:` it is a work-in-progress limit, which is
  the one planning constraint that is both genuinely useful and checkable from files.
- **`where:`** on `builtin.frontmatter` and `builtin.section` — apply a requirement only
  to documents in a declared state. "Finished work has evidence" and "answered questions
  record the answer" are the same check with different conditions.

Three layers in a row have now been built without touching the engine. The check
vocabulary is where the pressure lands instead, and each addition so far has been
general enough to be worth having on its own — which is the test for whether something
belongs in the core rather than in a layer.

## Choices worth recording

**`visual-qa` captures evidence and stores no baselines.** A pixel-diff suite fails on
every intentional change, so it needs a blessing workflow, and blessing goes reflexive
within about two weeks — at which point it catches nothing and costs everybody time. The
interesting failures are not pixel differences anyway: this is confusing, this breaks at
320px. A person looks at four screenshots. That works because looking is fast and
checking out a branch is not.

**`dev-loops` bounds attempts, and the gate decides completion.** An agent asked "are you
done?" eventually says yes; a test suite does not. Hitting the ceiling is reported as a
finding with the three likely causes, not as an instruction to raise the ceiling.

**`pm-loops` became `plan-hygiene`.** The idea as posed was a scheduled cadence; what
shipped is the same intent as failing checks. A weekly generated planning document is a
rhythm, and rhythms belong to a team — ilk's leverage is making a bad state fail, not
reminding somebody to look.

**`kanban` was deferred rather than deprioritised.** Its value is already here — the WIP
limit is in `plan-hygiene`, the derived view is `ilk blueprint next` — and what remains
is a rendering. Two overlapping planning layers is how a repository ends up with two
plans. It is worth building as a *view* over `blueprint`, owning no state of its own.

## The one thing that cannot be a layer

`mcp-servers` is blocked on a core change and should not be faked. Layers emit neutral
artifacts; only targets project them into agent-specific files, and there is no `mcp:`
artifact type. A layer shipping a literal `.mcp.json` would work for exactly one agent —
precisely the thing ilk exists not to do. Adding `mcp:` alongside `instructions:`,
`skills:` and `hooks:` is the clearest remaining gap in the core.
