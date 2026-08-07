# ilk

**A package manager for a repository's process, not its dependencies.**

Coding agents cannot inherit what was never written down. `ilk` manages the layers a
repository uses to work well with them — directory contracts, instructions, skills,
hooks, gates and checks — as versioned units you can add, upgrade and remove at any
point in a project's life.

```sh
ilk init
```

That gives you a project record, agent instructions, one validator, a session brief,
a pre-commit hook, and the skills an agent needs to operate ilk itself. No registry,
no network, nothing to choose.

---

## Why this exists

A generator answers "what should a repo look like on day zero", then leaves. Its later
improvements never reach you, and you cannot undo what it did.

`ilk` answers day *n*. Practice is added incrementally, upgraded deliberately, and
removed cleanly. When someone publishes a post about a new pattern on a Tuesday, a
repository can add it that afternoon and remove it on Thursday when it turns out not
to fit.

Three commitments constrain everything else:

- **Any coding agent.** The CLI is the interface; agent configuration is a generated
  projection of it. Anything that can run a shell command gets the whole feature set.
- **Useful alone.** `ilk init` with no arguments produces a better repository than the
  one before it, with no network access.
- **Community-extensible.** Publishing a layer is as cheap as publishing the blog post
  that described the idea: a git repo with a manifest.

## What it puts in your repository

Layers group the directories they introduce, so the root stays legible as you add more:

```
docs/            README.md is a generated index of what is below
  reference/     what is true now
  plans/         what we intend to build
  log/           what happened, dated
scratch/         rough notes, ungoverned and gitignored on purpose
```

Ordering lives in that generated index, not in the paths — no `00-`, `01-` prefixes to
renumber, and nothing for two independently published layers to collide over. Layers
that need somewhere new join an existing group (`docs/questions`, `docs/evidence`) or
declare their own; `infra/` and `ops/` are recognised alongside `docs/`.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/coflounder/ilk/main/install.sh | sh
```

or

```sh
npm install -g @coflounder/ilk
brew install coflounder/tap/ilk
go install github.com/coflounder/ilk/cmd/ilk@latest
```

## Use

```sh
ilk init                         # set the repository up
ilk brief                        # the packet a session should start with
ilk check                        # validate; every failure prints its fix
ilk status                       # what this repository has, and what has drifted

ilk search                       # layers you could add
ilk list --available             # layers you can add
ilk info gates           # what a layer contains, before adding it
ilk add gates --set test.command="go test ./..."
ilk rm gates             # removes exactly what it added

ilk add gh:someone/their-layer   # community layers, same interface
ilk agents add cursor            # generate config for another agent

ilk upgrade                      # merges layer changes into files you have edited
ilk update --path ../ilk         # rebuild ilk itself from a source checkout
ilk baseline list                # files exempt from a layer's checks, and why
```

Every command takes `--json`. Agents are first-class consumers of this tool, not an
afterthought.

## What a layer is

A directory with a `layer.yaml`. Everything else is optional.

```yaml
id: acme/quality-gates
version: 0.2.0
summary: Verification an agent cannot talk its way past.

requires: [test.command]          # a capability, not a layer name
provides: [gates.tests]

files:        [...]               # literal files, with an ownership mode
dirs:         [...]               # directory contracts
instructions: [...]               # always-on guidance, with a declared token budget
skills:       [...]               # detail, loaded only when its situation applies
checks:       [...]               # validators, each carrying its own fix
hooks:        [...]               # session-start, pre-commit, pre-push, …
commands:     [...]               # extend the CLI: `ilk quality-gates report`
```

Layers require **capabilities**, not each other. `gates` needs `test.command`;
anything that supplies it — your config, or another layer — satisfies that, so one gate
layer works in any language.

See [docs/reference/REF-writing-layers.md](docs/reference/REF-writing-layers.md) to write one, and
[docs/reference/REF-design-proposal.md](docs/reference/REF-design-proposal.md) for the reasoning behind the design.

## How rm can be safe

Every file ilk writes has an ownership mode, and the lockfile records a hash of what
ilk put there. That is the difference between "unchanged since I wrote it" (safe to
overwrite or delete) and "a human has edited this" (stop and say so).

| mode | ilk owns | on upgrade | on rm |
|---|---|---|---|
| `managed` | the whole file | overwrite | delete |
| `region` | a fenced block inside a file you own | replace the block | remove the block, keep the file |
| `create-only` | nothing after the first write | never touch | leave in place |
| `append-once` | a marked block, added idempotently | no-op | remove the block |

ilk also keeps a copy of what it last wrote, under `.ilk/base/`. That is what makes
an upgrade over a file you have edited a **three-way merge** rather than a refusal:
where your changes and the layer's touch different parts, both survive.

```
$ ilk upgrade
  ⇄ merge      AGENTS.md [instructions]                     acme/style
      merged acme/style's changes with yours
```

Where they genuinely collide, ilk refuses and names the lines. It never guesses —
a wrong merge is worse than a refusal, because a refusal is visible. You then pick:

| | |
|---|---|
| `--merge-markers` | write both versions into the file and resolve there |
| `--accept` | keep your version, and record it as ilk's new baseline |
| `--force` | take the layer's version, discarding yours |

`region` is the load-bearing mode. It means ilk can put guidance in your `AGENTS.md`,
your `.gitignore` and your CI config without ever owning them:

```markdown
# My project

Prose I wrote. ilk will never touch this.

<!-- ilk:begin layer=ilk/record region=instructions -->
…generated…
<!-- ilk:end layer=ilk/record region=instructions -->
```

Nothing is written until you have seen the whole plan:

```
$ ilk add gates

  + create     .github/workflows/ilk.yml                    ilk/gates
  + create     .git/hooks/pre-push                          target:git-hooks
  + block +    AGENTS.md [instructions]                     ilk/gates
  ~ block ~    AGENTS.md [skills]                           target:agents-md

  Apply? [y/N]
```

## Adding a layer to a repository with history

A layer governs what happens next, not what came before. When a layer's directory
contract lands on a directory that already has files — the common case for `docs/` —
those files are recorded as its baseline and exempted from its checks:

```
$ ilk init
  · 4 existing file(s) will be exempt from ilk/record's checks
      docs/reference/CONTRIBUTING.md, docs/reference/api_reference.md, …
      They predate the layer, so it does not judge them. `ilk baseline` to review.
```

Without this, `ilk init` in any repository with existing documentation opens with a
wall of failures about files nobody has touched, which is a good way to get a tool
deleted. Files added *after* that are governed normally.

The exemption is a ratchet, not an amnesty. It is recorded, visible in
`ilk baseline list`, and tightened one file at a time:

```sh
ilk baseline list                          # what is exempt, and from which layer
ilk baseline clear docs/api_reference.md   # conform it, then hold it to the rules
ilk init --no-baseline                     # or be held to them from the start
```

## Layers

`ilk init` gives you three, embedded in the binary so they need no network:

| Layer | What it does |
|---|---|
| `toolkit` | The skills an agent needs to operate ilk here — add, configure, apply, resolve a conflict, write a layer |
| `record` | The project record: what is true, what is intended, what happened, plus the checks that keep it honest |
| `gates` | Tests, lint and build wired into `ilk check`, git hooks and CI |

Nine more ship alongside in [`layers/`](layers/):

| Layer | Enforces |
|---|---|
| `blueprint` | Every spec belongs to an epic and a milestone that exist, and says what "done" means |
| `plan-hygiene` | A work-in-progress limit, active work with a named owner, finished work with evidence |
| `ask-human` | A blocking question an agent could not answer stops the checks until a person answers it |
| `dev-loops` | An agent runs until an objective gate passes, bounded by a ceiling — never until it says it is finished |
| `visual-qa` | Interface work shows what it looks like: screenshots against the acceptance criteria, no baselines |
| `compound-lessons` | Every lesson names the durable change it produced, so "we will be more careful" cannot pass as an outcome |
| `archive` | Superseded documents are archived rather than deleted, and nothing live may cite them |
| `gh-projects` | A GitHub Project made to match the plan, with an ambiguous match refused rather than guessed |
| `maintainer` | The receiving end: proposals from repositories using your layers, and a queue that fails when it stops being a promise |

```sh
ilk search planning
ilk info gh:coflounder/ilk/layers/blueprint
ilk add gh:coflounder/ilk/layers/blueprint --allow-exec
```

The index is embedded, so `ilk search` works offline. It lists what somebody
registered; it endorses nothing. `ilk info` shows what a layer writes, what it costs
in always-on context, and whether it runs code — read it before adding it.

## Keeping a tracker in agreement with the record

Most teams have a board somebody else reads. If it disagrees with the plan in the
repository, the plan stops being the source of truth no matter what a document claims.

```sh
ilk mirror plan gh-projects     # what would change on the board
ilk mirror apply gh-projects    # make the board match
```

It is `plan` and `apply` again, pointed at somebody else's system: nothing is written
until the whole plan has been seen. Three rules make that safe to run unattended.

**The record wins.** The tracker is made to match the markdown, never the reverse, so
"which one is right" is never a question. A board that has drifted is a change to push.

**An ambiguous match is refused, not guessed.** When a document's title could mean two
items, ilk names both and writes neither. A wrong link is silent and permanent — every
later sync writes to the wrong item, and nobody finds out until somebody reads the board
and does not recognise it. Linking a board that already has items goes through
`ilk mirror link`, which records the identity once so titles stop mattering.

**Nothing is deleted remotely.** An item no document claims is reported. Deciding it is
dead is a person's call.

ilk itself knows nothing about GitHub. A layer supplies three commands that normalise
the provider to `{id, title, status, url}`, which is the whole integration surface —
[`gh-projects`](layers/gh-projects/) is under 200 lines of shell, most of it error messages.

## Staleness is measured by coupling, not by a timer

A stale document is as dangerous as no document: an agent reads it confidently and
acts on something that stopped being true months ago. But a universal expiry cannot
work — a week is an eternity for a prototype and nothing at all for a stable
subsystem, and any fixed number is wrong for almost everybody. Worse, a timer teaches
people to bump a date to silence it, which is the one habit a documentation check
must never train.

So a document declares what it describes, and goes stale when *that* changes:

```yaml
---
id: arch-payments
title: Payments
updated: 2026-08-06
covers:
  - src/payments/**
  - migrations/*payment*
---
```

```
✗ record.stale   docs/reference/ARCH-payments.md
    11+ commits touched src/payments/** since this was last reviewed (2026-06-01);
    most recent: d9b7f8c "rewrite settlement flow", 4 days ago
    fix: Run `ilk record review <file>` …
```

This calibrates itself to a project's maturity without being told. Code nobody has
touched cannot make its documentation stale, however old that documentation is; code
rewritten yesterday makes it stale immediately. The project's own pace sets the pace
of review.

`ilk record review <file>` prints the commits and the diffstat under those paths,
then records that you looked — so the acknowledgement is informed rather than a date
you typed to make a check shut up.

Two failure modes are reported rather than passing quietly: a document with no
`covers:` (never stale, which is indistinguishable from always right), and a `covers:`
pattern matching nothing (worse — it looks like it is working). A document genuinely
not coupled to any path says so with `covers: []`, which is a decision a reader can
see rather than an oversight.

Tune it with `review_after_commits` per project, or per document when one is more
volatile than the rest. `max_age_days` adds an absolute backstop, off by default, for
documents that go stale because the world moved rather than the code.

## The flywheel

A layer that its adopters cannot improve decays. Somebody edits the managed file, the
edit works, the repository moves on, and upstream never finds out its content was
wrong — so the next hundred adopters make the same edit.

ilk is well placed to close this, because it already knows exactly what it delivered
and what the repository decided it should say instead. The difference is recorded, not
guessed at.

```sh
ilk contribute status           every layer with something to send back
ilk contribute gh-projects      draft the proposal
ilk contribute gh-projects --submit
```

**The evidence is gathered; the argument is not.** The diff comes out against the
layer's *own source path*, so a maintainer applies it without translating
`.claude/skills/x/SKILL.md` back to `skills/x.md` in their head. Alongside it go the
things a patch cannot carry: a default nobody kept, a check that could not run, an
exemption never cleared, and — from git — how many times the repository has come back
to the same file and how long the change has held. "Changed once, just now" and
"changed four times, held for 190 commits" are very different claims.

Two sections are left marked `TODO(you):` — what you needed, and why it is not
specific to your repository — and submission refuses while they stand. A maintainer
receiving diffs with no case attached learns to ignore the whole channel.

**Nothing leaves that should not.** A credential in a diff blocks submission outright:
a proposal is public and git history is permanent. Everything else is raised and not
enforced, and nothing is ever stripped — editing evidence on the way out changes what
upstream is being asked to judge.

**A patch that carries local values says so.** When the artifact was templated, the
diff would take this repository's directory names upstream with it. That is marked,
and the proposal goes as evidence for a change somebody makes properly at the other
end.

Each layer ships its own [`CONTRIBUTING.md`](layers/gh-projects/CONTRIBUTING.md),
printed when you draft, so you learn its standard before writing rather than in review.

### The other half

`ilk upgrade` is the return path. When a maintainer acts on a proposal, the improvement
three-way merges into every repository that had tuned the layer — including the one
that proposed it. That is the loop closing: a local fix stops being local, and stops
being yours to maintain.

The [`maintainer`](layers/maintainer/) layer is the receiving end. Proposals land as
documents, `ilk check` fails one that has no case or a verdict with no reasons, and the
open queue has a ceiling — because a queue is a promise, and past a certain length it
stops being one.

## Working with any agent

Layers never emit agent-specific files. They declare what an agent should know and do;
targets project that into whatever each agent reads.

| Neutral artifact | Where it lands |
|---|---|
| instructions | `AGENTS.md`, plus pointer stubs for `CLAUDE.md`, `.cursor/rules/`, `.github/copilot-instructions.md`, `GEMINI.md` |
| skill | `.agents/skills/<name>/SKILL.md`, mirrored to `.claude/skills/`; indexed in `AGENTS.md` for agents with no skill support |
| command | `ilk <layer> <command>`; thin slash-command wrappers where an agent has them |
| hook | git hooks (universal), plus native hooks where an agent has them |

Two rules keep that honest:

1. **Adapters only ever write `ilk hook run <event>`.** Adding a hook to a layer never
   rewrites an agent's settings file.
2. **Gates that matter are git hooks and CI.** Agent-native hooks make feedback faster;
   they are not the enforcement. An agent that ignores every instruction still cannot
   push past a failing gate.

`ilk doctor` reports where an agent cannot deliver an event, rather than leaving you to
assume a gate is running when it is not.

## The instruction budget

Unbounded agent instructions measurably make agents worse — published evaluations of
generated `AGENTS.md` files found lower task success and higher cost, mostly from
restating what the repository already showed.

So every layer declares the token cost of its always-on instructions, and `ilk check`
fails when the repository total crosses a ceiling. Detail belongs in skills that load
when their situation applies, not in every context window.

## Development

```sh
go test ./...                  # the contract lives in internal/engine/engine_test.go
go build ./cmd/ilk
ilk layer test ./layers/mine   # prove your layer's add/rm round trip
```

## Licence

MIT. See [LICENSE](LICENSE).
