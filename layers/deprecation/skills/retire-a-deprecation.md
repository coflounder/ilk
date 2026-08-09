# Retire a deprecation

Finish a deprecation: remove what it covers, then clear the notice.

## When

`deprecation.overdue` or `deprecation.done` is failing, or `ilk deprecation list`
shows something close to its date and somebody has to decide.

## See where everything stands

```
ilk deprecation list
```

```
OVERDUE   2026-06-01   {{ index .Caps "record.docs" }}/DEPRECATED-legacy-auth.md
due       2026-11-01   {{ index .Caps "record.docs" }}/DEPRECATED-v1-webhooks.md
done      2026-09-01   {{ index .Caps "record.docs" }}/DEPRECATED-batch-import.md
```

Three states, three different jobs.

## `done` — the removal already happened

Nothing the notice covers is left in the tree. This is the state nobody notices on
their own, which is why it is a check: the work finished, and the document describing
it stayed in current truth, where it now describes the past as the present.

Retire it:

```
ilk archive it {{ index .Caps "record.docs" }}/DEPRECATED-batch-import.md
```

If this repository has no archive, delete the file and record what happened:

```
ilk record log "removed the batch import path"
```

One thing to rule out first: the paths may be wrong rather than gone. A rename, a
moved directory, or a typo in `covers:` produces exactly the same reading. Look at
what the notice says it was about before you conclude the work is done — if the code
is still there under another path, fix `covers:` instead.

## `due` — the removal has not happened yet

Do the work. `covers:` is the scope: it names the paths that have to be empty, so it
doubles as the checklist. Remove them, and the notice moves to `done` by itself.

If callers outside this repository still depend on it, the deprecation is not really
`due` yet — it is waiting on somebody else, and that is worth writing into the
document while you can still see it.

## `OVERDUE` — the date passed and the code is still here

Two honest moves, and one dishonest one.

**Remove it.** This is what the date was for. `covers:` tells you exactly what is in
scope, and the check goes green the moment the paths are gone.

**Move the date, and say why.** Change `remove_after:` to a date somebody has actually
committed to, leave `announced:` alone, and add a line to the document saying what
changed and who agreed. The diff then shows a growing gap between the day this was
declared and the day it is now due — which is the only evidence anyone will have that
a deprecation is being carried rather than closed.

**Exempting it** is the dishonest one, and it is the move this layer cannot stop you
making. See below.

## The thing this layer does not solve

A deprecation becomes urgent on a date chosen months earlier, and it arrives while
somebody is doing something else. The check goes red, the removal is not five minutes
of work, and the cheapest way out is to make the check stop complaining — extend the
date by a year, or drop the notice entirely. Nothing here prevents that. `ilk check`
can make a state fail; it cannot make a person do the removal.

What the layer does instead is make the evasion cost a visible diff. Extending edits
a committed file, and `announced:` stays put beside it, so the interval is legible to
a reviewer in a way "we'll get to it" never was. Deleting the notice deletes the
document that said why the code exists. Both show up in review; neither is silent.

That is weaker than enforcement, and it is worth being clear that it is weaker. If
extensions in your repository are routine, the layer is telling you something true —
the dates are not being chosen by anybody who can commit to them — and the fix is
upstream of the check, in how the date gets picked. The `deprecate-something` skill is
about that.
