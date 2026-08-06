# Update the record

Bring the project record back in line with the code after a change. Do this as part
of the change, not as cleanup later — a doc that lags the code is the failure mode
this whole directory structure exists to prevent.

## When

- You shipped work that altered architecture, an interface, or operational behaviour.
- `ilk check` reported `record.stale` — enough commits have touched the paths a
  document `covers:` that nobody can any longer claim it has been verified.
- You are about to open a pull request. Reading what you shipped is not optional, and
  this is the moment you find out whether the docs still describe it.

## Procedure

1. **Find what moved.** For a stale-check failure, run
   `ilk record review <file> --show`: it prints the commits and the diffstat under
   that document's `covers:` paths since it was last read. For a change you are
   making now, `git diff --stat <base>` and ask which documents `covers:` those paths.

2. **Read the document against the code**, not against your memory of the code. The
   common failure is confirming a doc from the same mental model that wrote it.

3. **Decide which of three cases applies**, and act:

   | Case | Action |
   |---|---|
   | Still accurate | Run `ilk record review <file>` — it records the date for you, after showing you what you are signing off on. |
   | Now wrong | Fix the prose. Present tense, current state. Do not annotate the change inline — the log is where "it used to be X" lives. |
   | Now obsolete | Set `status: superseded`, link forward to whatever replaced it. Deleting it loses the reasoning that makes the replacement legible. |

4. **Write a log entry if something happened** that a future reader would need to
   reconstruct: `ilk record log "Moved retries into the gateway"`. A doc update alone
   records what is now true; the log records that it changed and why.

5. **Check the `covers:` list itself.** If the subsystem grew a new directory, or
   moved, the patterns need updating too — a pattern that matches nothing exempts the
   document from ever being checked again, which is worse than no check at all.

6. **Re-run `ilk check`.** It is the arbiter, not your judgement about whether you got
   everything. If it still fails, the failure names its own fix.

## What not to do

- Do not bump `updated:` to silence the check without reading the document. The whole
  point of `ilk record review` is that it shows you the commits first; skipping that
  and editing the date by hand costs you nothing now and costs whoever trusts the
  document next everything.
- Do not widen a `covers:` pattern to make a failure go away, and do not narrow one to
  dodge future failures. Either is a lie about what the document describes.
- Do not write the change into `plans/`. Plans describe intent; once the work is
  done, the truth belongs in `docs/` and the event belongs in `log/`.
- Do not create a new document when an existing one covers the area. Two documents
  describing the same subsystem is how a record stops being a source of truth.
