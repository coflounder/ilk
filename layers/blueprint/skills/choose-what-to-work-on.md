# Choose what to work on

Pick the next piece of work from the plan, rather than from memory or preference.

## When

A session starts with no assigned task, or somebody asks what to do next.

## Ask the record, not yourself

```
ilk blueprint next
```

It lists specs that are not done, grouped by milestone, with their acceptance-criteria
counts. This is derived from the plan documents every time it runs, so it cannot be
stale in the way a status report written last Tuesday is stale.

## Choosing between candidates

In order:

1. **Does anything block it?** A spec whose epic is `status: dropped`, or whose
   milestone has passed, is not ready however appealing it looks.
2. **Is it actually specified?** A spec with one vague acceptance criterion will cost
   more in clarification than in code. Improving the spec *is* the work in that case,
   and is worth saying out loud rather than starting and stalling.
3. **Earliest milestone first.** Milestones exist to order work; ignoring them makes
   them decorative.
4. **Then whatever unblocks the most other work.**

## When nothing is ready

That is a finding, not an inconvenience. A repository where every spec is done, or
every remaining spec is unspecified, has run out of prepared work — and the useful
response is to say so, not to invent a task.

Report it plainly: what is left, why none of it is ready, and what would make it
ready. An agent inventing work to fill a session is how a plan stops describing the
project.

## Before starting

Read the spec in full, then its epic, then any decision documents it links. `ilk brief`
gives you the current state of the repository, including whether validation is clean —
starting work on a repository that is already failing its checks means you will not
know which failures are yours.

## What not to do

- Do not pick work because it is interesting when the plan says something else is
  next. If the plan is wrong, change the plan — that is a legitimate move and takes
  one commit.
- Do not start a spec whose acceptance criteria you cannot evaluate. Ask, or write
  better criteria first.
- Do not do work that is in no spec at all without saying so. Small fixes found along
  the way are fine and normal; a day of unplanned work is a plan that has stopped
  describing the project.
