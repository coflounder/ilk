# Layers

Layers that ship alongside ilk but not inside it. Add one with:

```sh
ilk add gh:coflounder/ilk/layers/blueprint
```

Pin a version with `@v0.2.0`. Nothing here is added by default — `ilk init` gives you
the built-in layers only, so a fresh repository still needs no network.

## What is here

| Layer | Enforces |
|---|---|
| [`blueprint`](blueprint/) | Every spec belongs to an epic and a milestone that exist; every spec says what "done" means; every epic states an outcome |
| [`plan-hygiene`](plan-hygiene/) | A work-in-progress limit, active work with a named owner, finished work with evidence |
| [`ask-human`](ask-human/) | A blocking question an agent could not answer stops the checks until a person answers it — and where it can name the options, it offers them with what each would mean |
| [`dev-loops`](dev-loops/) | An agent runs until an objective gate passes, bounded by a ceiling — never until it says it is finished |
| [`visual-qa`](visual-qa/) | Interface work shows what it looks like: screenshots against the acceptance criteria, no baselines |
| [`compound-lessons`](compound-lessons/) | Every lesson names the durable change it produced, so "we will be more careful" cannot pass as an outcome |
| [`archive`](archive/) | Superseded documents are archived rather than deleted, and nothing live may cite them |
| [`gh-projects`](gh-projects/) | A GitHub Project made to match the plan — the record is the source of truth, and an ambiguous match refuses rather than guesses |
| [`maintainer`](maintainer/) | Proposals from repositories using your layers arrive as reviewable documents; one nobody can judge fails the checks rather than sitting in a queue |
| [`autoresearch`](autoresearch/) | A finding about the world outside the repository cites its sources, says what would change its conclusion, and expires on a date |
| [`deprecation`](deprecation/) | A deprecation carries a removal date the checks hold you to, and says when the removal is actually finished |
| [`pr-prep`](pr-prep/) | A pull request description is derived from the plan — outcome, criteria, evidence — rather than written from memory |
| [`secrets`](secrets/) | A credential is caught while the fix is still cheap, and there is a written answer for the day one leaks anyway |
| [`pulumi`](pulumi/) | `preview` is agent work and `up` is human work; every stack has a project file and none carries a plaintext secret |
| [`routine`](routine/) | Scheduled work is a document with an owner, a budget and a review date, and what actually runs it is generated from that document |
| [`gauntlet`](gauntlet/) | Finished work names who judged it, what it was compared against, and the largest thing still wrong — the builder does not grade its own homework |
| [`html-wireframe`](html-wireframe/) | Interface work is sketched and agreed before it is built, as one HTML file that opens from disk with no network and no build |

### Built in

Three more layers ship *inside* the binary, so `ilk init` works with no network at all.
They are ordinary layers — same manifest format, same `ilk layer test` contract, same CI
— and they live elsewhere only because `go:embed` cannot read above its own package
directory:

| Layer | Source |
|---|---|
| [`record`](../internal/builtin/layers/record/) | `internal/builtin/layers/record/` |
| [`gates`](../internal/builtin/layers/gates/) | `internal/builtin/layers/gates/` |
| [`toolkit`](../internal/builtin/layers/toolkit/) | `internal/builtin/layers/toolkit/` |

Editing them takes effect only after `go build` — a stale binary silently uses the old
templates, which is the single most common way to waste an hour in this repository.

The first three community layers plus `compound-lessons` and `archive` come from the
[MetaHarness essay](https://www.tenex.co/blog/building-an-ai-native-sdlc) — the project
record, the connected plan, the escalation path, lessons that compound, and an archive
the tooling refuses to write into. `dev-loops` and `visual-qa` are the two failure modes
that essay does not cover: an agent that stops because it believes it is finished, and
an agent that cannot see what it built. `gh-projects` is the third — the record is only
the source of truth if the tracker everyone else reads agrees with it.

## Layers that run code

`blueprint`, `archive`, `ask-human`, `dev-loops`, `gh-projects`, `maintainer`,
`autoresearch`, `deprecation`, `pr-prep`, `secrets`, `pulumi`, `routine` and
`html-wireframe` ship shell commands, so adding them requires consent:

```sh
ilk info gh:coflounder/ilk/layers/dev-loops     # read it first
ilk add gh:coflounder/ilk/layers/dev-loops --allow-exec
```

`plan-hygiene`, `visual-qa`, `compound-lessons` and `gauntlet` are entirely declarative
and need no flag.

---

## Every layer here takes proposals

Each declares a `contribution:` block and ships its own `CONTRIBUTING.md` saying what
it wants to hear about. From a repository that has one:

```sh
ilk contribute status                    what there is to send back, across every layer
ilk contribute <layer>                   draft it, with the evidence gathered
ilk contribute <layer> --submit          open it upstream
```

The guidelines are per-layer on purpose. What is useful to know about `dev-loops` (what
the gate was, and why it was the right completion condition) has nothing in common with
what is useful to know about `record` (which `covers:` globs measured the wrong thing).
A single repository-wide CONTRIBUTING file would say neither.

CI fails if a layer here has no `contribution:` block, or names guidelines that are not
there. A layer its adopters cannot improve decays, and that is too easy to not notice.

---

## Not built yet

These are specified rather than vague — each has a decided shape, and what is missing is
the work, not the design. Contributions welcome; so is disagreement with the shape.

The order these get built in is
[docs/plans/PLAN-layer-queue.md](../docs/plans/PLAN-layer-queue.md), along with the
reasoning behind it. Its first two items — check assertions, and the credential story —
are done, which is why the entries below are the ones that are left.

### `migrations` — an applied migration is immutable

Hash every migration ilk has seen applied, and fail when one of them changes. Require
either a down migration or an explicit, written statement that this one is
irreversible.

**Decided.** Editing an already-applied migration is the kind of mistake that works
perfectly on the machine that made it and corrupts every other environment, which is
exactly the shape of error an agent makes and no test catches.

**Blocked on a question rather than on work.** The immutability rule is universal;
where migrations live and what "applied" means is not. It likely needs a capability
supplying both the directory and the applied set, which is a wider interface than any
current layer asks for. Worth settling before writing it, because a layer that only
fits one ORM is a layer that should have been a script.

**First consumer** would be a drizzle project, which is a reasonable place to find out
whether the interface generalises.

### `release-notes` — assembled from the record, not from memory

Derive a changelog from `docs/log/` entries and the plans marked accepted since the
last release, rather than having somebody reconstruct it from commit messages.

**Decided.** The record already holds what happened and what was accepted; a release
note written by hand is that same information typed a second time, less accurately.
Pairs with `pr-prep`, which does the same derivation one scope down.

**Shape:** a command that assembles the notes for a version range, a check that a
tagged release has notes, and a convention for the one thing derivation cannot supply
— which changes a reader actually needs to act on.

**Not before `pr-prep`**, which is smaller and establishes whether deriving prose
from the record produces something anybody wants to read.
### `brainstorm` — probably should not be its own layer

The idea: force divergent thinking before convergent — several genuinely different
options considered before one is chosen, rather than the first idea becoming the plan by
default.

**The problem with the obvious version.** A check that counts alternatives in a document
is trivially gameable, and worse, gaming it is the path of least resistance: an agent
asked for three options will produce one option and two strawmen, pass the check, and
have learned that the ritual is the requirement. A check that can be satisfied without
doing the thing teaches people to not do the thing.

**What is actually checkable** is not the number of alternatives but the *link between a
decision and what it rejected*. That artifact already exists: `docs/reference/DEC-*.md`,
written by the `write-decision` skill. A decision naming the alternatives it rejected and
why is durable, useful to a future reader, and hard to fake usefully — because the
rejected options have to be specific enough to argue with.

**So the recommendation is:** extend `write-decision` and `compound-lessons` with a
"rejected alternatives" section and a check that a `DEC-` document has one, rather than
build a `brainstorm` layer. Divergent thinking itself belongs in `scratch/`, which is
ungoverned on purpose — that is what the ungoverned annex is *for*, and putting process
around it would leak governance into the one place the record deliberately keeps free
of it.

**Build the standalone layer only if** somebody can name a check for it that cannot be
satisfied by writing three paragraphs nobody believes.

### `mcp-servers` — unblocked; the core change landed

Declare the MCP servers a repository expects, and project them into each agent's config
the way instructions and hooks already are.

**Was blocked on:** the `mcp:` artifact type, which now exists. A layer declares a
server once under `mcp:` and each configured target projects it — `.mcp.json` for
Claude Code, `.cursor/mcp.json` for Cursor — as an `ilk mcp run <name>` entry resolved
from the manifest at start time, with `requires_env:` carrying the credential story.
See [writing a layer](../docs/reference/REF-writing-layers.md).

**What remains** is only deciding whether a standalone layer bundling common servers
earns its place, or whether `mcp:` blocks inside the layers that need them (`codegraph`,
`linear-mirror`) cover every real case. No further core work is required.

### `linear-mirror` — unblocked, and nothing is left but the work

Keep the record and Linear in agreement: derive issues from specs, report where the two
disagree, and refuse to write when a spec could plausibly match two issues.

**No longer blocked on design.** `ilk mirror` is the generalised surface, and
`gh-projects` proves it against a real tracker. A Linear layer is now three shell
commands normalising to `{id, title, status, url}` — identity, diffing, ambiguity
refusal and plan-then-apply are already core and already tested.

**No longer blocked on credentials either.** The part `gh-projects` sidestepped by
leaning on `gh` — reading an API token — is answered by `requires_env:` on a check: the
variables that carry a credential are named, their presence is tested and their values
are never read, and a check whose credential is absent skips with a reason rather than
failing as if the board were empty. `pulumi` is the first layer to use it.

**What is left is the work.** Copy `layers/gh-projects/`, point it at Linear's API, and
name the token variable in `requires_env:` — nothing in core needs to change.

### `codegraph` — pick the indexer, wrap it

A structural map of the codebase an agent can query instead of re-deriving it by
grepping every session.

**Decided:** wrap an established indexer rather than writing one.
[CodeGraph](https://github.com/topics/code-index) is the current pick — tree-sitter
parsing into embedded SQLite, served over MCP, local-first with no code egress, and by
some distance the most adopted tool in the category since early 2026. A published study
across 31 repositories reported roughly 10× fewer tokens and 2.1× fewer tool calls for
agents given a tree-sitter knowledge graph over MCP.

**Shape:** a layer that declares the indexer as a capability (`codegraph.command`),
keeps the index out of git, refreshes it on a hook, and documents in `AGENTS.md` that the
map exists and how to ask it things. Depends on `mcp-servers` for the good version.
The open question this closes is the one in the roadmap: how a layer ships a check
needing a real parser without every layer shipping a binary. The answer is that it does
not — it requires a capability and lets the project supply the parser.

### `kanban` — deliberately deferred

A board derived from the record: columns from `status:`, WIP limits, cycle time.

**Deferred because** most of its value is already here and the rest is a competing
model. `plan-hygiene` gives the WIP limit — the one board mechanic that changes
behaviour — and `ilk blueprint next` gives the derived view. What remains is a
rendering, and two overlapping planning layers is how a repository ends up with two
plans.

Worth building if it becomes a *view* over `blueprint` rather than an alternative to it:
`ilk kanban board` rendering columns and cycle time from the same documents, owning no
state of its own.

### `pm-loops` — became `plan-hygiene`

The idea as posed was a scheduled cadence. What shipped is the same intent as failing
checks rather than a calendar: a WIP limit, ownership, and evidence for completed work.

The scheduling half was dropped on purpose. A weekly generated planning document is a
rhythm, and rhythms are a team's to keep — ilk's leverage is in making a bad state
*fail* rather than in reminding somebody to look. The cadence version remains a
reasonable layer for a team that wants it; it just should not be the default.

### Also considered

- **`incident`** — a runbook contract and a post-incident path that feeds
  `compound-lessons`. Only meaningful for a project with production, which most adopters
  will not have on day one.
- **`llm-wiki`** — a maintained knowledge layer over the codebase. Overlaps `codegraph`
  and the record; worth revisiting once `codegraph` exists, since the interesting version
  is the synthesis on top of the index, not another index.

---

## Adding one

Publishing a layer should cost about as much as publishing the post that described the
idea. There is no server and no account:

1. Write it — [docs/reference/REF-writing-layers.md](../docs/reference/REF-writing-layers.md) is the guide,
   and `ilk layer new` scaffolds one.
2. `ilk layer test ./your-layer` must pass. It adds the layer to a throwaway
   repository, applies twice to prove idempotency, removes it, and asserts the repository
   came back. A layer that cannot be cleanly removed is one nobody can safely try.
3. Add an entry to [`internal/registry/registry.yaml`](../internal/registry/registry.yaml)
   so `ilk search` can find it, and open a pull request.

Your layer does not have to live in this repository. `ilk add gh:you/your-layer` works
whether or not it is indexed; the index only makes it discoverable.

CI runs `ilk layer validate` and `ilk layer test` against everything in this directory on
every push, so these are held to the same standard as any published layer.
