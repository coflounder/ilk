# Ask a human

Escalate a decision you cannot make for yourself, without stalling and without
guessing.

## When

- A choice would be **expensive to reverse** — a schema, a public interface, a
  dependency, anything that gets built on before anybody notices it was wrong.
- The requirements **genuinely contradict** each other, and picking one silently means
  somebody's stated need is being dropped without them knowing.
- You need **access or authority you do not have** — a credential, a production
  action, permission to change something outside the scope you were given.
- You notice you are **about to invent an assumption**. That noticing is the signal.
  It is easy to miss because inventing an assumption feels like making progress.

## When not to

Escalating is cheap but not free: every question spends somebody's attention, and a
repository full of questions nobody needed to answer trains people to skim them.

Do not ask when:

- A sensible default exists and the choice is easy to change later. Pick it, say you
  picked it, move on.
- The answer is in the repository. Read `docs/`, the decision documents, the spec.
  "What does this project use for X" is almost never a question for a person.
- You are asking for reassurance rather than information. "Is this approach okay?" with
  no specific uncertainty behind it is a request for permission, not a question.

The test: **would two reasonable people, with everything in this repository in front
of them, disagree about the answer?** If not, decide.

## How

```
ilk ask-human open "Should retries live in the gateway or the client?"
```

Then fill in the sections, and put real effort into this one:

> **What I would do without an answer**

State your best guess and what it would cost if wrong. This is what makes the question
cheap to answer — somebody can reply "yes, do that" in four seconds instead of writing
an essay. A question with no proposed answer is work you have handed to somebody else.

Set `blocking:` honestly. `true` means work is actually stopped; `ilk check` will fail
while it is open, so it appears in every session's brief until somebody deals with it.
If you can work around it, say `false`. **Marking everything blocking is how the signal
stops meaning anything.**

## Then keep going

Do everything the answer does not block, and say clearly what you left out and why.

Stopping entirely on one open question is almost always the wrong response — it turns
a five-minute reply into a blocked day. The exception is when proceeding under any
assumption would be unsafe, or would make the work useless if the assumption is wrong.
That is a small set, and you should be able to say which case you are in.

## When the answer arrives

The person answering edits the file and sets `status: answered`. If you are the one
picking it up:

1. Read the answer against what you already built. Work done under the wrong
   assumption needs revisiting, not just redirecting from here.
2. If the answer is a decision with lasting consequences, promote it to a
   `docs/DEC-*.md` and link it from the question. Answers buried in a questions folder
   get re-asked.
3. Close the loop: the question stays as the record of why the decision was needed.

## What not to do

- Do not ask and then proceed on your guess anyway without saying so. That is the
  worst of both: somebody's attention spent, and the assumption baked in regardless.
- Do not batch a week of questions into one document. One question, one file — they get
  answered at different times by different people.
- Do not delete a question that stopped mattering. Set `status: withdrawn` and say why.
  "We asked this and it became irrelevant" is information the next person wants.
