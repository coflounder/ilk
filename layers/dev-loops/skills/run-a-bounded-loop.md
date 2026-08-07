# Run a bounded loop

Drive an agent repeatedly at one task until an objective gate passes.

## When looping is the right tool

The work must be **convergent** and **mechanically checkable**:

- Making a failing test suite green.
- Migrating call sites after an interface change.
- Fixing lint or type errors across a codebase.
- Filling in a generated stub until it compiles and passes.

The common shape: you know exactly what "done" looks like, a machine can tell you
whether you are there, and each attempt gets closer.

## When it is the wrong tool

- **Design work.** A loop cannot converge on a decision. Twenty attempts at "make the
  architecture better" produce twenty rewrites, not one good one.
- **Anything where the gate is weak.** If the gate is a test suite with one assertion,
  a loop will find the shortest path to satisfying that assertion, which is usually
  not the code you wanted. The loop optimises the gate; the gate had better be the
  thing you care about.
- **Work needing a decision.** If the second attempt fails for the same reason as the
  first, another eight will too. Escalate: `ilk ask-human open "..."`.

## How

```
ilk dev-loops run 'your-agent-command'
ilk dev-loops run 'your-agent-command' --gate 'go test ./...' --max 5
```

The default gate is `ilk check`, which is usually right: it runs this repository's
tests, lint and build, and validates the record as well.

## The two rules that make this safe

**State lives in the repository.** Files, diffs, git history — not in the conversation.
An attempt ending is then not a loss, and the next attempt starts from real state
rather than a summary of it. If your command depends on remembering the previous
attempt, it will not survive the loop.

**Completion is decided by the gate, never by the model.** This is the whole point. An
agent asked "are you done?" will eventually say yes; a test suite will not. The gate
must be something that cannot lie.

## After it stops

**If the gate passed:** read the diff in full before trusting it. A loop converges on
the gate, which is not the same as converging on what you wanted — the classic result
is a test suite made green by weakening the tests. Check that no assertion was
loosened, no test skipped, no error swallowed.

**If it hit the ceiling:** that is a finding, not an instruction to raise the ceiling.
Read the last gate log and say which case you are in:

- The gate cannot be satisfied by the change being attempted — the task was misjudged.
- The task needs a decision rather than another attempt — escalate it.
- The work was never convergent — looping was the wrong tool, and the attempts are
  probably worth discarding rather than building on.

Raising `--max` and running again is occasionally right and usually a way to spend a
lot of tokens on a problem that needed a person after the second attempt.

## What not to do

- Do not loop with the gate disabled or weakened to "make progress". A loop against a
  gate you have softened is a machine for generating plausible wrong code.
- Do not loop on a dirty working tree you have not read. You will not be able to tell
  your changes from the loop's.
- Do not leave a loop running unattended against anything with side effects outside
  the repository — sending, deploying, writing to a shared system. The ceiling bounds
  attempts, not blast radius.
