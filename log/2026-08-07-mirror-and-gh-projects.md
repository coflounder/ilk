---
date: 2026-08-07
title: Writing to somebody else's system, and the first tracker to use it
---

# Writing to somebody else's system, and the first tracker to use it

Shipped `ilk mirror` in core and `gh-projects` as the first layer to use it. The record
in `plans/` can now be made to agree with a GitHub Project, and ilk still does not know
what a GitHub Project is.

## Where the line went

This is the first time ilk writes anywhere other than the repository, and the split
follows the same rule as everything else: core owns what is true of every tracker, a
layer owns what is true of one.

Core owns identity (a frontmatter key it writes and nothing else touches), diffing,
plan-then-apply, and every refusal. A layer supplies three commands and normalises its
provider to `{id, title, status, url}`. That is the entire integration surface —
`gh-projects` is under 200 lines of shell, most of it error messages — and it is why
`linear-mirror` is no longer a design problem. What still blocks it is credentials:
`gh-projects` sidestepped that by leaning on `gh auth`, and a provider with its own API
token needs a decided answer for where the token lives and what absence looks like.

## The refusals are the feature

Two are worth stating, because both are cases where doing something reasonable is worse
than doing nothing.

**An ambiguous title is refused and both candidates named.** A wrong link is silent and
permanent: every later sync writes to the wrong item, and nobody finds out until somebody
reads the board and does not recognise it. There is no partial credit here, so matching is
case- and whitespace-insensitive and no fuzzier than that — anything more produces
confident wrong links.

**A status the board does not offer is refused, and the options listed.** Inventing an
option puts a value on somebody else's board that nobody chose; skipping silently lets the
two drift while reporting success. Naming what the field does offer makes it a
ten-second fix.

Nothing is ever deleted remotely. An item no document claims is reported, because
deciding it is dead is a person's call.

## Three bugs the end-to-end run found that the unit tests did not

The unit tests were green and the feature was broken in three ways. All three were found
by adopting the layer into a scratch repository and running it against a fake `gh`, which
is an argument for doing that before believing a test suite.

**`match:` was never rendered.** The layer declares `match: "{{ .Vars.match }}"`, and
`readDocs` compiled that string as a regex directly. It matched nothing, so the mirror saw
zero documents — which looks exactly like a mirror with nothing to do. The tests had all
passed a literal pattern.

**A document with unreadable frontmatter was skipped in silence.** The `match` pattern had
already said the document belonged to the mirror; dropping it there took it off the tracker
without saying so, and the tracker looked complete while missing it. Now it is a named
error with a fix, the same call made for a missing directory.

**A created item landed with no status.** `Apply` recovered the status by searching the
action's change list, and a create has nothing to diff against, so the field was always
empty. Every new item needed a second run to acquire its status. The status now rides on
the action itself.

The first and third are the same mistake in different clothes: deriving a value that was
already known rather than carrying it.

## A fake provider, committed

`layers/gh-projects/test/` holds a fake `gh` and a suite that runs the provider script
against it, wired into CI. It cannot prove the real API calls are right — only a real
project can do that, and this environment has no route to one. What it does prove is that
the contract with ilk holds and that every refusal still refuses, and the fake is strict
about unrecognised calls so a changed invocation fails loudly rather than silently.

**The script has never run against a live GitHub Project.** That is the honest state of it.

## Smaller things

- Command-based checks now get `ILK_VAR_*` and `ILK_LAYER` in their environment, like
  layer commands already did. Previously a check could only reach its layer's
  configuration by interpolating it into the command string, which works until the first
  value with a space in it.
- The roadmap listed tracker sync as out of scope. It came off the list deliberately
  rather than quietly: the boundary that mattered was keeping provider knowledge out of
  the binary, and that one held.
