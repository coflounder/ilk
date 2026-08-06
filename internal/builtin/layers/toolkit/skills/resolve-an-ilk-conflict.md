# Resolve an ilk conflict

Decide what happens when ilk wants to write a file that somebody has edited since ilk
last wrote it.

## When

A plan shows `CONFLICT`; a merge reports conflicting hunks; or `ilk check` reports
`ilk.conflicts`.

## What a conflict actually means

ilk records a hash of everything it writes. A conflict means the file on disk is not
what ilk left there — so writing would destroy somebody's work. **Nothing is written.**
The rest of the plan still applies; only the conflicting artifact is skipped.

This is not an error state. It is ilk declining to make a decision that is yours.

## First: work out whose change it is

Read the note on the conflict line, then look at the file. There are three situations
and they have different answers.

### 1. The edit belongs to you, and it should not be in ilk's block

Somebody wrote something useful inside an `ilk:begin` / `ilk:end` block. Generated
blocks are replaced wholesale, so that content will be lost every time.

**Move it outside the markers**, then apply normally. Prose outside the fence is
yours for ever; inside it is the layer's. This is almost always the right answer for
`AGENTS.md`.

### 2. The edit belongs to you, and it should stay where it is

You deliberately diverged from what the layer produces and want to keep that.

```
ilk apply --accept
```

Your version stays on disk and becomes ilk's new baseline. Later layer changes will
merge on top of it rather than fighting it. This is the honest way to say "we do it
differently here".

### 3. The edit was accidental, or the layer's version is better

```
ilk apply --force
```

Your version is discarded. **Read the file first** — `--force` is unrecoverable
outside git, and "I assumed it was scratch" is how work disappears.

## When a merge itself conflicts

If both you and the layer changed the *same* lines, ilk reports the hunks and refuses:

```
! CONFLICT   AGENTS.md [instructions]              acme/style
    2 hunks conflict with your edits, around line 4, 19 — resolve them by hand,
    re-run with --merge-markers to write both versions into the file, --accept to
    keep yours, or --force to take acme/style's
```

To resolve in the file:

```
ilk apply --merge-markers
```

Both versions are written with `<<<<<<<` / `=======` / `>>>>>>>` markers. Then:

1. Open the file and choose. Do not keep both halves because choosing is hard — a
   document that says two contradictory things is worse than either version.
2. Delete all three marker lines.
3. `ilk apply --accept` — this records your resolution as the new baseline.

Step 3 is the one people skip. Without it, ilk still believes the file is unresolved,
and `ilk check` will keep reporting `ilk.conflicts`.

## The one thing that is always wrong

Do not delete the file to make the conflict go away. ilk will recreate it, you will
have lost whatever was there, and the underlying disagreement is still unresolved.

## Verifying

```
ilk check --only ilk.drift,ilk.conflicts
```

Both should pass. If `ilk.conflicts` still fails, marker lines are still in a file
somewhere — it names which.
