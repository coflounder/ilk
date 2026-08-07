# Keep the plan honest

Review the plan for the failures that accumulate quietly, until it has stopped
describing what is actually happening.

## When

A hygiene check fails; before a planning session; or when somebody asks a question
about status that the plan cannot answer.

## The three failures, and what they actually mean

### Work in progress above the limit

More things are `status: active` than anybody is progressing. This is the most
expensive failure and the least visible one, because every individual item looks
reasonable — it is only the count that is wrong.

Fix it by finishing or by demoting, not by raising the limit. For each active item ask:
**has this moved in the last week?** If not, it is not active, it is abandoned in
place. Set it back to `proposed` and say why in a line. That is not a defeat; it is the
plan becoming true again.

Raise `wip_limit` only if you have concluded the team really does run more work in
parallel than the limit allows. A limit you raise whenever you hit it is not a limit.

### Active work with no owner

Nobody is accountable, so nothing stalls visibly. Add `owner:` naming a person or a
team — not "the team", which is nobody.

If no owner can be named, that is the finding: the work is not really active. Demote it.

### Finished work with no evidence

A completion claim nobody can check. Add an `## Evidence` section: the commands run and
what they said, the pull request, and for each acceptance criterion the one line that
settles it.

If you cannot reconstruct the evidence, say so in the document rather than inventing
it. "Marked done on 2026-06-02; evidence not recorded at the time" is honest and useful.
A fabricated evidence section is worse than an empty one, because it is trusted.

## Doing the review

```
ilk check --only hygiene.wip,hygiene.owner,hygiene.evidence
ilk blueprint next        # if the blueprint layer is adopted
```

Work through the failures in that order — the WIP limit first, because demoting stalled
work usually resolves several of the others at once.

Then read the plan as somebody outside the team: **does this describe what is
happening?** The checks catch structural failures. They cannot catch a plan that is
structurally perfect and describes a project that stopped existing two months ago.

## What not to do

- Do not mark things done to clear the board. A false `done` is worse than an honest
  `abandoned` — one is a lie in the record, the other is information.
- Do not add an owner by picking whoever is least likely to object.
- Do not treat these checks as a reason to plan less. The failure they catch is a plan
  that describes intentions nobody holds any more, and the answer to that is a smaller
  true plan, not a larger vague one.
