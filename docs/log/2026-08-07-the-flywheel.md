---
date: 2026-08-07
title: The flywheel — contributing back, and the layer that receives it
---

# The flywheel — contributing back, and the layer that receives it

Until now traffic was one-way. A layer was published, repositories adopted it,
`ilk upgrade` merged improvements down. A layer could only ever learn what its author
already knew, while the repositories using it in anger knew more and had nowhere to
put it.

`ilk contribute` is the way up, and `ilk/maintainer` is the receiving end.

## The thing that made this cheap

ilk already knew what it delivered. `Delivered` in the lockfile and the base store
behind it were built for three-way merge — for carrying a layer's changes *down* into
a repository that had tuned it. Read in the other direction, the same two facts are
exactly a contribution: here is what the layer produced, here is what this repository
decided it should say instead. The divergence is recorded, not guessed at.

That is the second time the base store has paid for something it was not built for.
Worth noticing when deciding what to keep.

## Evidence, not argument

`ilk contribute` gathers everything a machine can and refuses to write the part it
cannot.

Gathered: the diff, rendered against the layer's **own source path** so a maintainer
never translates `.claude/skills/x/SKILL.md` back to `skills/x.md` by hand; whether the
divergence was *agreed*, meaning ilk asked and the answer was not the layer's; how many
commits have touched the file and how long the change has held. "Changed once, just now"
and "changed four times, held for 190 commits" are very different claims and it would be
misleading to report either as a bare number.

Alongside the diff go the signals a patch cannot carry — a default nobody kept, a check
that could not run, an exemption never cleared, an artifact deleted outright. One
repository overriding a default is a preference; upstream seeing the same override in
proposal after proposal is a default that is simply wrong, and that pattern is only
visible if each one is reported.

Not gathered: whether the edit is a fix everybody needs or a quirk of one repository.
Two sections are left marked `TODO(you):` and submission refuses while they stand. A
generated paragraph guessing at somebody's reasoning would read like an argument and
carry none, and a maintainer receiving diffs with no case attached learns to ignore the
whole channel — which costs far more than any single proposal is worth.

## Three refusals

**A credential blocks submission outright.** A proposal is public and git history is
permanent; rotating a leaked token is somebody's afternoon and not publishing it is one
regex. The pattern list is deliberately narrow — a screen that fires on anything
resembling a hex string trains people to pass whatever flag silences it.

**Nothing is ever stripped.** An absolute path or a repository name is raised and not
enforced, because often that context *is* the evidence. Editing evidence on the way out
would change what upstream is being asked to judge.

**A templated artifact goes as evidence, not as a patch.** If the layer's source does
not survive verbatim in what was delivered, a patch built from the delivered text would
carry this repository's values upstream too. ilk will not guess its way back through a
template: it says so, and the change gets made by hand at the other end.

## Four bugs the end-to-end run found

The unit tests were green and it was broken four ways. All four were found by adopting
a layer into a scratch repository and actually tuning it. This is now twice in a row.

**Skills were invisible to the layer that shipped them.** A skill is written by an agent
target, so the lockfile records it against `target:agents-md`. Going by ownership meant
the single most valuable thing an adopter can improve — the wording of a skill — could
not be contributed. Attribution now runs across the whole lockfile.

**A skills-only layer could not contribute at all.** `toolkit` writes no files of its
own, so it has no lockfile entry, so there was nothing to scan. Fixed by not going via
the layer's entry.

**The target's generated frontmatter made every skill look templated.** Portability was
a byte comparison against the source, and a target prepends its own header. Now the
source has to survive verbatim as a suffix, and the header is dropped from both sides of
the patch — nobody upstream can change it anyway.

**`--since=@0` silently returned no commits.** git parses `@0` as *now* rather than as
the epoch, so asking for every commit touching a path got none — indistinguishable from
a path nothing has ever touched. `record.stale` always passes a real timestamp so it was
unaffected, but the helper was wrong for anybody else. Fixed at the source with a
regression test.

## The receiving end

`ilk/maintainer` is a layer, adopted by the repository that publishes layers. Proposals
land in `proposals/` as documents, so upstream's own `ilk check` runs on them in CI — a
proposal nobody can validate is one somebody has to read carefully by hand.

Its checks say four things. A proposal names its layer and its origin. One arriving with
the contributor's sections unwritten fails rather than being reviewed. One marked
reviewed records a verdict and its reasons — a proposal that looks handled and is not is
worse than an open one, because nobody looks at it again. And the open queue has a
ceiling, because a queue is a promise and past a certain length it stops being one.

The `review-a-proposal` skill carries the rubric, and its most important line is that
the failure mode is not accepting a bad change. It is the contributor concluding that
sending things here is not worth the effort.

## Per-layer guidelines

Every one of the twelve layers now declares `contribution:` and ships its own
`CONTRIBUTING.md`, printed when somebody drafts a proposal so they learn the standard
before writing rather than in review.

They are per-layer on purpose. What is useful to know about `dev-loops` — what the gate
was and why it was the right completion condition — has nothing in common with what is
useful about `record` — which `covers:` globs measured the wrong thing. One
repository-wide file would say neither. CI fails a layer with no `contribution:` block
or guidelines that are not there.

## Smaller things

- **`builtin.forbid`** joins the check vocabulary: text that must not survive into a
  finished document, each pattern carrying its own reason. The general case is a
  placeholder — nothing structural catches it, because the heading is present and the
  frontmatter is valid and only the words give it away. Pressure landing on the check
  vocabulary rather than the engine is now four batches running.
- **Layer subcommands are rendered.** A check's `run:`, a mirror's provider commands and
  a contribution's submit command all went through the template; `ilk <layer> <command>`
  did not, so a subcommand could not reach its own layer's variables. It showed up as
  `{{ .Vars.proposals_dir }}` in output.
- `ilk-propose.sh` resolves the upstream URL through `gh` rather than assembling
  `https://github.com/...`, which makes it work against an enterprise host and, not
  incidentally, testable against a local bare repository.

## What is not proven

`ilk-propose.sh` has a suite in `internal/builtin/layers/toolkit/test/` that runs the
real git operations against local bare repositories with only `gh` faked. It proves the
contract, the commit, the branch point and the refusals. It cannot prove the `gh`
invocations are the ones the real `gh` wants, and **the script has never opened a pull
request against GitHub.**
