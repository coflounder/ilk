What this layer wants from a proposal:

  - Say what happened on the day the check fired. This layer's whole bet is that a
    failing check is a better reminder than a comment; the interesting evidence is
    what the failure actually caused — a removal, an extension, or an exemption.
    The third is the one worth knowing about and the one least often reported.
  - If a notice was extended, show the diff. How far the date moved, and whether
    anything in the document changed with it, is the measurement this layer cannot
    take for itself.
  - If `covers:` could not express what the deprecation was about, say what it was.
    A deprecation spread across two repositories, or one that is about a call
    pattern rather than a path, is a limit of the mechanic rather than a bug in it —
    and the shape of what is missing is the proposal.
  - If a check fired on something legitimate, show the document.

## What this layer does not solve

A deprecation becomes urgent on a date chosen months earlier, and it arrives while
somebody is doing something else. When removal is not five minutes of work, the
cheapest way to make `ilk check` green is to move the date or delete the notice.
Nothing here stops that, and it would be dishonest to describe the layer as though it
did — a check can make a state fail, it cannot make a person do the removal.

What the layer does is make the evasion cost a visible diff: `announced:` never moves,
so an extension widens a gap a reviewer can see, and deleting the notice deletes the
document that explained why the code exists. Both land in review. That is weaker than
enforcement.

Two designs were considered and left out of 0.1.0:

  - **Failing on the extension itself** — reading git history for a `remove_after:`
    that has moved, and requiring a recorded reason. It is checkable, and it is the
    strongest version of this idea. It was left out because a first extension is
    often legitimate and a check that fires on the legitimate case teaches people to
    ignore it; the useful version fires on the *second* extension, and that needs a
    shape for "how many times" that has not been designed yet.
  - **A baseline, as `ilk/record` uses** — an exemption that is recorded, visible and
    shrinkable rather than silent. This is probably right eventually, and it is the
    open question the spec named. It is not here because the baseline mechanism
    exempts files that predate a layer, and an overdue deprecation is the opposite
    case: a file the layer has been watching all along.

A proposal that settles either of those is more valuable than a proposal that adds a
check.

## Interactions worth knowing about

`ilk/record` is the usual provider of `record.docs`, and two of its checks meet these
documents:

  - **`record.naming`** requires `^[A-Z]{2,6}-`, and `DEPRECATED-` is ten letters, so
    a notice using the default prefix fails it. Set `prefix: DEPR-` for this layer, or
    argue upstream that the grammar should admit a longer prefix — the record layer
    takes proposals too.
  - **`record.coverage`** reports a `covers:` pattern that matches no tracked file as
    an accident. For a finished deprecation that is not an accident, it is the point —
    and `deprecation.done` is telling you to retire the document, which resolves both
    complaints at once.
