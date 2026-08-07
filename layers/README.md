# Layers

Layers that ship alongside ilk but not inside it. Adopt one with:

```sh
ilk adopt gh:coflounder/ilk/layers/blueprint
```

Pin a version with `@v0.2.0`. Nothing here is adopted by default — `ilk init` gives you
the built-in layers only, so a fresh repository still needs no network.

## What is here

| Layer | Enforces |
|---|---|
| [`blueprint`](blueprint/) | Every spec belongs to an epic and a milestone that exist; every spec says what "done" means; every epic states an outcome |
| [`plan-hygiene`](plan-hygiene/) | A work-in-progress limit, active work with a named owner, finished work with evidence |
| [`ask-human`](ask-human/) | A blocking question an agent could not answer stops the checks until a person answers it |
| [`dev-loops`](dev-loops/) | An agent runs until an objective gate passes, bounded by a ceiling — never until it says it is finished |
| [`visual-qa`](visual-qa/) | Interface work shows what it looks like: screenshots against the acceptance criteria, no baselines |
| [`compound-lessons`](compound-lessons/) | Every lesson names the durable change it produced, so "we will be more careful" cannot pass as an outcome |
| [`archive`](archive/) | Superseded documents are archived rather than deleted, and nothing live may cite them |

The first three of these plus `compound-lessons` and `archive` come from the
[MetaHarness essay](https://www.tenex.co/blog/building-an-ai-native-sdlc) — the project
record, the connected plan, the escalation path, lessons that compound, and an archive
the tooling refuses to write into. `dev-loops` and `visual-qa` are the two failure modes
that essay does not cover: an agent that stops because it believes it is finished, and
an agent that cannot see what it built.

## Layers that run code

`blueprint`, `archive`, `ask-human` and `dev-loops` ship shell commands, so adopting
them requires consent:

```sh
ilk info gh:coflounder/ilk/layers/dev-loops     # read it first
ilk adopt gh:coflounder/ilk/layers/dev-loops --allow-exec
```

`plan-hygiene`, `visual-qa` and `compound-lessons` are entirely declarative and need no
flag.

---

## Not built yet

These are specified rather than vague — each has a decided shape, and what is missing is
the work, not the design. Contributions welcome; so is disagreement with the shape.

### `mcp-servers` — needs a core change first

Declare the MCP servers a repository expects, and project them into each agent's config
the way instructions and hooks already are.

**Blocked on:** this cannot be a layer as things stand. Layers emit neutral artifacts;
only *targets* project them into agent-specific files, and there is no `mcp:` artifact
type. `.mcp.json`, `.cursor/mcp.json` and the rest are different files with different
shapes, so a layer shipping one literal file would work for exactly one agent — the
thing ilk exists not to do.

**Shape:** add `mcp:` alongside `instructions:`, `skills:` and `hooks:` in the manifest;
teach each target to render it. Then a layer can say "this project talks to Linear and
Postgres" once. Perhaps 150 lines of core plus per-target rendering. It is the most
clearly correct item on this list.

### `linear-mirror` — needs a secrets and network story

Keep the record and Linear in agreement: derive issues from specs, report where the two
disagree, and refuse to write when a spec could plausibly match two issues.

**Blocked on:** ilk has never made a network call or read a credential, and both are
consequential enough to design rather than bolt on. The essay is emphatic about the
shape — you see the full plan before anything executes, applying is a separate
deliberate step, and an ambiguous match refuses and names both candidates. That is
`ilk plan` / `ilk apply` pointed at somebody else's system, which is a good sign the
model fits.

**Shape:** a layer providing `ilk linear plan` and `ilk linear apply`, reading a token
from the environment, never from the repository. The same design generalises to Jira and
GitHub Projects, so the first one should be built as though the second exists.

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

### `html-wireframe` — a design-before-code artifact

A static HTML wireframe produced and agreed *before* implementation, linked from the
spec, so disagreement is cheap while it is still only a wireframe.

**Shape:** a `wireframes/` directory contract; a check that a spec with `ui: true` and
`status: proposed` links a wireframe; a skill on writing one that provokes useful
disagreement rather than approval. Pairs with `visual-qa` — the wireframe is what the
spec agreed to, the evidence is what was built, and having both makes the comparison
somebody's job rather than nobody's.

**Not started because** `visual-qa` covers the more expensive failure. Building the
wrong thing beautifully is worse than building the right thing and checking it late, but
only just.

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

- **`pr-prep`** — assemble a pull request description from the spec, the acceptance
  criteria and the evidence, so the description is derived rather than written from
  memory. Small, useful, no blockers. Probably next.
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

1. Write it — [docs/REF-writing-layers.md](../docs/REF-writing-layers.md) is the guide,
   and `ilk layer new` scaffolds one.
2. `ilk layer test ./your-layer` must pass. It adopts the layer into a throwaway
   repository, applies twice to prove idempotency, drops it, and asserts the repository
   came back. A layer that cannot be cleanly removed is one nobody can safely try.
3. Add an entry to [`internal/registry/registry.yaml`](../internal/registry/registry.yaml)
   so `ilk search` can find it, and open a pull request.

Your layer does not have to live in this repository. `ilk adopt gh:you/your-layer` works
whether or not it is indexed; the index only makes it discoverable.

CI runs `ilk layer validate` and `ilk layer test` against everything in this directory on
every push, so these are held to the same standard as any published layer.
