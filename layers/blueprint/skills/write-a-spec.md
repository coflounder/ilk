# Write a spec

Turn an intention into something an agent can execute and a reviewer can accept or
reject.

## When

Starting a new piece of work; breaking an epic into deliverable slices; or a spec has
failed one of the blueprint checks.

## The shape

```
ilk record new plans "Move retries into the gateway"
```

Rename the generated file to `SPEC-move-retries-into-the-gateway.md`, then fill in
the frontmatter that connects it to the rest of the plan:

```yaml
---
id: spec-move-retries-into-the-gateway
title: Move retries into the gateway
status: proposed        # proposed | active | done | dropped
updated: 2026-08-06
epic: epic-reliability  # must resolve to an EPIC document
milestone: m3-hardening # must resolve to a MILESTONE document, or `backlog`
---
```

Both references are checked. A spec pointing at a milestone nobody wrote is exactly
the drift the blueprint exists to catch — it looks like planned work and is not.

## Acceptance criteria are the load-bearing section

Everything else in a spec is context. The criteria are what turn "done" from a claim
into something somebody else can settle.

Write each one so a person who did not write the code could decide it. The test:
**could two people disagree about whether it is met?** If yes, it is not a criterion
yet.

| Not a criterion | A criterion |
|---|---|
| Retries work correctly | A request failing with 503 is retried three times with backoff, asserted in `gateway_test.go` |
| Performance is acceptable | p99 latency for `/checkout` stays under 400ms in the load test |
| The docs are updated | `ARCH-gateway.md` describes retry ownership and passes `ilk check` |
| Error handling is robust | A malformed payload returns 400 with the field named, never 500 |

Name the command, the behaviour or the artifact that settles it. If the criterion
cannot be settled without asking its author, rewrite it.

## Sizing

A spec should be finishable. If it has more than a handful of criteria, or the
criteria describe several unrelated capabilities, it is an epic wearing a spec's
frontmatter — split it and let the epic hold them together.

The opposite mistake is worse in practice: a spec so small it is a task. If the
acceptance criteria are "the function exists", you are writing a to-do list, and the
plan will stop being readable long before it stops being accurate.

## Plans are allowed to change

Detailed and revisable are not in tension. When the work teaches you something,
change the spec and say why — that is the system working. What is not allowed is the
spec quietly diverging from what is being built, because then neither document nor
code can be trusted.

If the approach changes materially after work starts, record the decision in
`docs/DEC-*.md` and link it. The spec says what is being built; the decision says why
it stopped being the other thing.

## Before you call it ready

```
ilk check --only blueprint.epic,blueprint.milestone,blueprint.acceptance
```

And read it once as somebody who has not been in the conversation that produced it.
Most unbuildable specs are perfectly clear to their author.

## What not to do

- Do not write acceptance criteria after the implementation, chosen to match what you
  happened to build. That is a description, not a commitment.
- Do not use `milestone: backlog` to dodge the check when the work is actually
  scheduled. The exemption exists for genuinely unscheduled work.
- Do not restate the architecture in the spec. Link to it. Two copies drift, and the
  spec is the one that will be wrong.
