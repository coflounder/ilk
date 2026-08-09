# Draft a pull request

Assemble the description from the document the work came from, then edit it. The
order matters: starting from the record and cutting is a different act from starting
from the diff and remembering.

## When

You are about to open a pull request; somebody asked you to summarise a branch; or
`prprep.criteria-settled` or `prprep.evidence-checkable` failed.

## Procedure

1. Find the document the work came from.

   ```
   ilk pr-prep draft            # lists the plan documents, with their status
   ```

   If there is no document, that is the first finding, not a reason to skip this. Say
   so in the description in one line — a reviewer reading an undocumented change
   deserves to know it is undocumented — and write the document if the change is
   large enough to deserve one.

2. Draft from it.

   ```
   ilk pr-prep draft SPEC-webhooks > /tmp/body.md
   ```

   The body goes to stdout; anything the command has to say about the document goes
   to stderr, so the file is clean. Read the stderr remarks — each one is a gap in
   the record that the description just made visible.

3. Edit the body, subtracting rather than adding. Cut criteria that this pull request
   does not settle and say which request will. Cut evidence that has gone stale. What
   survives is a description a reviewer can check line by line.

4. Add the one thing the record cannot know: what a reviewer should look at first.
   Two sentences naming the file or the decision that carries the risk. This is the
   only part worth writing from memory, because it is the only part that is about the
   change rather than about the intent.

5. Open it.

   ```
   gh pr create --title "$(ilk pr-prep draft SPEC-webhooks --title)" --body-file /tmp/body.md
   ```

## When a check fails

**`prprep.criteria-settled`** — a document at `status: done` has an unticked box. One
of two things is true, and only you know which: the criterion is settled and nobody
ticked it, or the work is not done. Ticking a box you have not settled is the failure
this exists to prevent, so establish the answer before you touch the file. If the
criterion has been deferred, move it into a follow-up document and link it — deleting
it is how a commitment quietly stops existing.

**`prprep.evidence-checkable`** — the evidence section reads as evidence and is not.
Replace each bullet with the thing itself: the command in backticks and the line it
printed, the link to the run, the path to the test. If you cannot reconstruct what was
run, write that instead of inventing something. "Marked done on 2026-06-02; evidence
not recorded at the time" is honest and useful; a plausible-sounding command nobody
ran is worse than an empty section, because it will be believed.

## What not to do

- Do not summarise the diff. The reviewer has the diff. What they do not have is what
  the change was supposed to achieve and how you know it did.
- Do not paste the whole document. The description is the part of the record a
  reviewer needs in front of them, not a copy of it — link the rest.
- Do not write acceptance criteria that exist only in the description. They are
  invisible to every later change, and the first person to break one will never learn
  that it was a criterion.
- Do not regenerate over an edited description. The command is a starting point; once
  a human has been through it, running it again throws their subtraction away.
