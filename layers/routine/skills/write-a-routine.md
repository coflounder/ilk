# Write a routine

Decide whether work belongs on a schedule, and write it down if it does.

## First, try not to

A routine is work nobody is watching. Almost everything that looks like a routine is
better as something that fires when a person causes the problem, while the fix is
still cheap and the author still has the context:

| Instead of a routine that... | Use |
|---|---|
| looks for a problem in the code | a check, run by `ilk check` and by CI |
| catches something before it lands | a hook — `pre-commit` if the fix is cheap, `pre-push` if it is not |
| re-derives something from the repository | a command somebody runs when they need it |
| reminds a person to do a thing | a date on the document that thing is about |

That last one is the trap worth naming. A routine that opens an issue saying "review
the dependency policy" is a calendar with extra steps; `review_by:` on the policy
document does the same job with nothing running.

A schedule earns its place when **the trigger genuinely lives outside the repository
and outside anybody's session**: a vendor publishes advisories on their clock, a
certificate expires on its own, a queue drains overnight, a report has to exist by
Monday whether or not anyone committed last week.

## What a routine has to say

```yaml
---
id: dependency-audit
title: Audit dependencies for published advisories
status: active                # active | paused
schedule: "0 4 * * 1"         # five cron fields, UTC
command: npm audit --audit-level=high
budget: 15                    # minutes, then it is killed
owner: platform              # a person or a team, never "the team"
review_by: 2027-02-08         # when somebody decides this is still worth running
---
```

Then three sections: what it is for (the outcome, not the command), why it has to be
scheduled rather than checked, and what happens when it fails.

### The budget

Pick the number that is clearly too small to be a runaway and clearly big enough for a
normal run — usually two or three times the slowest run you have seen. It is killed at
the ceiling, not warned. A routine with a generous budget and an agent behind it is how
a weekend's tokens are discovered on Monday.

There is no default worth trusting here, which is why the check refuses a routine that
does not name one.

### The owner

A name, so failure has somewhere to go. `owner: the team` is the same as no owner: it
means whoever notices, and on a scheduled job nobody notices.

### The review date

Six months is the default `ilk routine new` writes. It exists for the failure mode
nothing else in a repository can see: a routine that has *succeeded* pointlessly every
night for a year. A failing routine gets fixed because it is loud. A pointless one is
silent for ever, and only a date will surface it.

## Putting it on the schedule

```sh
ilk routine sync        # writes the runner file from the documents
```

The documents are the source; the runner file is a projection of them.
`routine.current` fails when the two disagree, so sync and commit in the same change as
the routine. Editing the generated file directly is edited away on the next sync.

## What GitHub's scheduler actually promises

If `runner: github`, the projection is a workflow, and it inherits GitHub's terms —
worth knowing before you rely on one:

- Scheduled workflows run **from the default branch only**. A routine on a feature
  branch does not run, and testing one means `workflow_dispatch` or merging it.
- Runs are **dropped, not queued**, when the scheduler is busy. `0 * * * *` is a
  popular minute; `17 * * * *` is not.
- Scheduling **stops after 60 days** with no repository activity, silently.
- The clock is **UTC**, with no daylight saving. A routine that must happen at 9am
  local moves twice a year, and the honest fix is to pick a time where it does not
  matter.

Anything that must not be missed needs a runner that promises more than this — which
is what `runner: cron` on a machine you control is for.

## Running one by hand

```sh
ilk routine run dependency-audit
```

Same path the scheduler takes, budget included. Do this before committing a new
routine: a routine whose first real run is unattended at 4am is one you have not
tested.

## Pausing rather than deleting

`status: paused` keeps the document and drops it from the projection, so the reason it
existed survives. Delete it when the answer is "we were wrong about needing this" —
and say so in the commit, because the next person will have the same idea.
