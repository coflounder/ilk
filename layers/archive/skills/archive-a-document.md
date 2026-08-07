# Archive a document

Retire a document that is no longer true, without losing the reasoning in it.

## When

A document has been superseded; an approach was abandoned; or you are about to delete
something from the record.

That last one is the important trigger. **Deleting is almost always the wrong move.**
The reasoning behind a decision that was later reversed is what makes the reversal
legible — without it, the next person sees only the current answer and has no way to
know the alternatives were considered, so they get proposed again.

## How

```
ilk archive it {{ index .Caps "record.docs" }}/ARCH-old-gateway.md arch-new-gateway
```

The second argument is the id of whatever replaced it. It is optional and worth
supplying: it turns a dead end into a signpost for whoever finds the document later.

The command sets `status: superseded`, stamps `archived:` with today's date, records
`superseded_by:` if you gave one, and moves the file into the archive directory using
`git mv` so history follows it.

## Then fix what pointed at it

```
ilk check --only archive.no-live-links
```

Live documents may not link into the archive. A current document citing an archived
one presents the past as the present, which is the exact failure archiving exists to
prevent. For each broken reference, either:

- point it at the replacement, or
- restate the point inline if it was small, or
- delete the reference if it was only ever context.

## What the archive is for, and what it is not

Nothing in the archive is checked — not naming, not frontmatter, not freshness. It is
history, and history does not get corrected. Do not tidy it, do not update it when the
world moves on, and do not link out of it into live documents (those links will rot,
and nobody will fix them because nobody reads the archive looking for problems).

The archive is where you go when you ask "why on earth is it like this" and the
current documents do not say.

## When deleting is right

Genuinely: when the document never contained reasoning. A stub, a duplicate, a file
created by mistake, a template nobody filled in. If a reader six months from now would
learn nothing from it, deleting is honest and archiving is hoarding.

The test is not "might this be useful" — almost anything might. It is **"does this
explain a choice somebody could otherwise undo by accident?"**
