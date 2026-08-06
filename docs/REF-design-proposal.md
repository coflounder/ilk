---
id: ref-design-proposal
title: "Design proposal: a layer manager for AI-native repos"
status: current
updated: 2026-08-06
---

# ilk — a layer manager for AI-native repos

## 1. What the source material actually argues

The Tenex "Building an AI-Native SDLC" essay (MetaHarness) makes one structural claim
and eight principles that follow from it.

The claim: *agents cannot inherit what was never written down.* Agile could leave project
state implicit because humans carried it between days. Agents can't, so execution capacity
now scales faster than the systems that preserve context. The response is to make process
state a first-class, versioned, checkable artifact living next to the code.

The eight principles, and what each one implies for tooling:

| # | Principle | Mechanizable as |
|---|---|---|
| 1 | Context as code | A governed directory contract + naming grammar + an ungoverned scratchpad |
| 2 | Plans are detailed, and allowed to change | Schemas over plan documents; referential integrity between them |
| 3 | Every agent starts senior | A derived session-start packet ("benevolent prompt injection") |
| 4 | Skills ship like software | Versioned, testable, installable instruction units |
| 5 | One record, many views | A machine interface — structured output, not "read the files and guess" |
| 6 | Let agents check their own work | Validators whose errors name the fix precisely enough to self-heal |
| 7 | Status is derived, not reported | Everything computed from the record; disagreement is an error you can point at |
| 8 | Lessons compound for everyone | A lesson becomes a check / a skill / a command — and *travels* |

Two details are the load-bearing ones, and are easy to miss:

- **The fence rule.** Decide which parts of a document a machine may rewrite and mark them
  off. "Retrofitting this means untangling every document where a sync has already
  overwritten someone's prose." This is a mutex expressed as syntax, and it must exist on
  day one.
- **"Start with three folders, not seven."** MetaHarness's own authors say their taxonomy
  is not the point and that you should resist designing one before you've used one.

Principle 8 is the one that has no tooling today. Tenex encodes lessons into *their*
skills, for *their* org. There is no mechanism for a lesson published on the open internet —
a blog post about Ralph loops, or LLM wikis — to become an installable, versioned,
removable change to a repo's process. **That gap is what `ilk` is for.**

## 2. Thesis

> `ilk` is a package manager for a repository's *process*, not its dependencies.

`django-cookiecutter` answers "what does a good repo look like on day zero?" It is a
one-shot: it generates, then it leaves, and the template's later improvements never reach
you. `ilk` answers "what does a good repo look like on day *n*?" — where practice is
adopted incrementally, upgraded, and dropped, at any point in a project's life.

The unit is a **layer**: a versioned bundle of any combination of directory contracts,
skills, hooks, gates, checks, scripts, schemas, and subcommands, that can be adopted and
dropped cleanly.

Three commitments follow, and they constrain everything below:

1. **Any coding agent.** Portability is achieved by making the CLI the interface and agent
   config a *generated projection* of it. Anything that can run a shell command gets the
   full feature set.
2. **Immediately useful alone.** `ilk init` with no arguments must produce a repo that is
   better than the one before it, with the bare minimum config, and no registry access.
3. **Community-extensible.** Publishing a layer must be as cheap as publishing the blog
   post that described the idea — a git repo with a manifest.

### What ilk is not

Not a project tracker, not an agent runner, not a prompt library, not an MCP server (v1),
not opinionated about your language or test framework. Integrations (Linear, GitHub
Projects, Jira) are layers, not core. Core stays about files, contracts, and checks.

## 3. The core model

### 3.1 Layer

A layer is a directory with a manifest. Everything else is optional.

```yaml
# layer.yaml
id: ilk/compound-memory
version: 0.3.1
summary: Durable project record, derived session brief, staleness checks.
facets:
  arc: context           # discovery only — see §7
  kind: record
ilk: ">=0.4 <1.0"

requires: [vcs.git]                              # capabilities, not layer names
provides: [record.docs, record.log, context.brief]

variables:
  docs_dir: { default: docs, prompt: "Where does current truth live?" }
  log_dir:  { default: log }

files:
  - src: AGENTS.section.md.tmpl
    dest: AGENTS.md
    mode: region                # fenced, ilk-owned block inside a human-owned file
    region: compound-memory
  - src: docs/README.md.tmpl
    dest: "{{ .docs_dir }}/README.md"
    mode: create-only           # seed once, never touch again
  - src: skills/write-decision/SKILL.md
    dest: "skill:write-decision"     # projected per agent target — see §5
    mode: managed                    # fully ilk-owned; upgrades overwrite

checks:
  - id: record.frontmatter
    run: "ilk-check-frontmatter --dir {{ .docs_dir }}"
    fix: "Add required keys (id, status, updated). See {{ .docs_dir }}/README.md"
  - id: record.stale
    kind: builtin.stale-docs
    args: { dir: "{{ .docs_dir }}", against: "src/**", max_lag_days: 30 }

hooks:
  - event: session-start
    run: "ilk brief --json"
  - event: pre-commit
    run: "ilk check --only record.frontmatter"

commands:
  - name: log                 # → `ilk compound-memory log "..."`, or `ilk log` if unambiguous
    summary: Append a dated entry to the log
    run: ./bin/log.sh
```

**Capabilities over names.** A layer `requires: [test.command]` rather than requiring
`ilk/pytest`. The repo satisfies it either from `ilk.yaml` directly or from any adopted
layer that `provides` it. This keeps the quality-gates layer from caring what language you
write, and lets community layers compose with layers they've never heard of.

**File modes** are the whole game for clean adopt/drop/upgrade:

| mode | write | on upgrade | on drop |
|---|---|---|---|
| `managed` | ilk owns the file entirely | overwrite | delete |
| `region` | ilk owns a fenced block inside a human file | replace block | remove block, keep file |
| `create-only` | seed a starting point | never touch | leave in place |
| `append-once` | idempotent append keyed by marker | no-op | remove marked lines |

`region` is the fence rule from the essay, generalized from planning docs to every file
ilk touches — `AGENTS.md`, `.gitignore`, CI workflows, `settings.json`. It is why
`ilk drop` can be non-destructive, and it exists in v1 or not at all.

### 3.2 Repo state

```
ilk.yaml            # desired state — human/agent editable, committed
.ilk/lock.json      # resolved versions, source digests, per-file provenance hashes
.ilk/cache/         # fetched layer sources (gitignored)
```

```yaml
# ilk.yaml
version: 1
targets: [claude-code, cursor, codex]     # agent projections to generate
capabilities:
  test.command: "pytest -q"
  lint.command: "ruff check ."
layers:
  ilk/compound-memory: { version: 0.3.1, vars: { docs_dir: docs } }
  ilk/quality-gates:   { version: 0.2.0 }
  gh:ghuntley/ralph:   { version: 2.1.0 }
```

`adopt` and `drop` are sugar over editing `ilk.yaml` and reconciling. The reconcile is
Terraform-shaped, which is what makes this safe:

```
$ ilk adopt quality-gates

  ilk/quality-gates 0.2.0  (source: registry, sha 9f2c1ab)

  + create   .ilk/checks/gates.yaml
  + create   scripts/gate.sh                    (executable)
  ~ region   AGENTS.md            [+14 lines]   block: quality-gates
  ~ region   .claude/settings.json [+1 hook]    (generated — do not edit)
  + skill    review-changes        → .agent/skills/, .claude/skills/
  + checks   gates.tests, gates.lint, gates.acceptance-criteria
  ! requires test.command — satisfied by ilk.yaml

  Apply? [y/N]
```

The lockfile stores a content hash per written file. That's how `upgrade` knows whether
you edited a `managed` file (three-way merge, or a `.ilk-new` sidecar rather than silent
clobbering), and how `drop` knows whether removing a file would destroy your work.

### 3.3 Checks

Every layer contributes checks to one aggregated runner:

```
$ ilk check
✗ record.stale            docs/ARCH-overview.md unchanged since src/payments/ changed 41d ago
                          fix: review and touch `updated:`, or `ilk record ack ARCH-overview`
✗ gates.acceptance        plans/PS-portal.md ticket 6 → milestone M3, which does not exist
                          fix: relink to an existing milestone, or remove the reference
✓ record.frontmatter      12 documents
2 failing, 1 passing      (ilk check --json for machine output)
```

Principle 6's design rule is non-negotiable: **every check failure carries a fix hint
precise enough for an agent to self-heal and re-run.** A check that only says "invalid" is
a bug.

## 4. The CLI surface

Small, verb-first, and every command supports `--json`. Agents are first-class consumers,
not an afterthought.

```
ilk init [--profile minimal|standard|metaharness]
ilk adopt <layer>[@version]      ilk drop <layer>       ilk upgrade [layer]
ilk plan                         ilk apply              ilk status
ilk check [--only id] [--fix]    ilk brief              ilk doctor
ilk search <term>                ilk info <layer>       ilk list
ilk agents add <target>          ilk agents sync
ilk layer new|test|publish       ilk hook run <event>
ilk <layer> <command> ...        # layer-provided subcommands
```

Three deserve a note:

- **`ilk brief`** is principle 3. It assembles the session-start packet from the record:
  what the repo is, the directory contract, current check status, unblocked work, recent
  decisions, and the commands available to verify claims. It's what the session-start hook
  calls, and what a human pipes into any agent that has no hook support. This is the
  single highest-value command in the tool.
- **`ilk doctor`** reports which declared hook events each configured target actually
  supports, so degradation is visible rather than silent.
- **`ilk hook run <event>`** is the *only* entrypoint any agent adapter ever writes. Adding
  a hook to a layer never requires touching an adapter.

## 5. Portability: how "any coding agent" actually works

This is where comparable tools fail, so it's worth being explicit.

**Layers never emit agent-specific files.** They emit neutral artifacts; `ilk` projects
them per target:

| Neutral artifact | Projection |
|---|---|
| instructions | `AGENTS.md` (canonical — Linux Foundation AAF spec, 60k+ repos), plus pointer stubs for `CLAUDE.md`, `.github/copilot-instructions.md`, `.cursor/rules/*.mdc`, `GEMINI.md` |
| skill | `.agent/skills/<name>/SKILL.md` canonical → `.claude/skills/`; for targets without skill support, an index line in `AGENTS.md` ("when doing X, read `<path>`") |
| command | `ilk <layer> <cmd>` is universal; adapters emit thin `.claude/commands/*.md` wrappers that shell out to it |
| hook | git hooks + CI as the universal substrate; agent-native hooks (`.claude/settings.json`) as an optimization |

Two rules keep this honest:

1. **The CLI is the interface; agent config is a projection.** Adding a target is writing
   an adapter, never touching a layer. `ilk agents sync` regenerates all projections and
   is safe to run any time.
2. **Deterministic gates never rely on agent cooperation.** A gate that matters is a git
   hook or a CI job. Agent-native hooks make the feedback faster; they are not the
   enforcement. An agent that ignores its instructions still can't push past the gate.

There is a real constraint worth designing against: an AGENTS.md that grows without bound
makes agents *worse*. Published evaluation of LLM-generated AGENTS.md files found a 2%
drop in success rate and a 23% cost increase, mostly from restating what the repo already
shows. So `ilk` enforces an **instruction budget**: each layer declares a token cost,
`ilk check` fails when the aggregate crosses a configurable ceiling, and layers are
expected to put detail in on-demand skills rather than always-on instructions.

## 6. The default: what `ilk init` gives you with no registry

Taking the essay's own "if we were starting over" advice literally — three folders, not
seven, plus the scratchpad sooner than you expect:

```
docs/          what is true now
plans/         what we intend to build
log/           what happened
scratch/       ungoverned annex (gitignored by default)
AGENTS.md      short; the contract + the commands, not a handbook
ilk.yaml
.ilk/
```

Plus, from one command: one validator (`ilk check` — frontmatter schema, referential
integrity between plans and docs, staleness against the code), one derived view
(`ilk brief`), the fence rule active in every file ilk touches, and a git pre-commit hook
wired to the validator.

That's principles 1, 3, 6, and 7 in a `curl | sh` and one command, with no network calls
after install and no layers adopted. Everything past that is opt-in.

## 7. Taxonomy: your instinct to distrust "SDLC arcs" is right

Scoping layers by SDLC arc is a good *discovery* aid and a bad *type system*. Ralph loops
are execution + verification + memory at once; an LLM wiki is memory + documentation +
onboarding. Forcing a tree means arguing about where things go instead of shipping them.

Proposal: **layers are flat and globally named. Arcs are tags.** Two orthogonal facets,
both advisory, both used only for search and curation:

- `arc`: context · planning · execution · quality · release · operations
- `kind`: record · process · gate · harness · integration · target

Then ship **profiles** — named sets of layers — for the cookiecutter-shaped experience:
`ilk init --profile metaharness` adopts the full record + gates + brief stack;
`--profile minimal` is §6. Profiles are one-shot expansions, not a dependency, so nothing
is trapped inside one.

## 8. Community extensions

**Sources.** Local paths, git URLs, `gh:owner/repo` shorthand, and a default registry that
is just an index file in a git repo mapping names to repos and tags. No server, no
hosting, no account. `ilk search` reads the index; `ilk adopt gh:someone/their-layer@v2`
skips it entirely.

**Authoring.** `ilk layer new` scaffolds a layer. `ilk layer test` adopts it into a
temporary repo from a fixture, then asserts the resulting file tree, the checks that
register, and that `drop` restores the fixture exactly. Principle 4 — "skills ship like
software" — applied to layers themselves: versioned, tested against fixtures, released
deliberately.

**Trust.** Layers can write files and run scripts, which is a supply-chain surface, so it
is designed in rather than bolted on:

- Layers are **declarative-first**. Templates, checks, and hooks cover most of what a
  layer needs without arbitrary code.
- `ilk plan` shows *every* file write and *every* script before anything executes.
- Executable content requires explicit consent at adopt time (`--allow-exec`), recorded in
  the lockfile.
- The lockfile pins commit SHAs and content digests; `ilk check` fails on drift.
- A `--pure` mode allows file rendering only, for repos that want a hard guarantee.

**The test of success:** someone publishes a post about a new pattern on a Tuesday, ships
`gh:them/that-pattern` the same day, and a repo adopts it with one command and drops it
cleanly on Thursday when it doesn't fit.

## 9. Technical choices

| Decision | Recommendation | Why |
|---|---|---|
| Language | **Go**, single static binary | Runs in any repo regardless of language; no runtime to install; ~5ms startup matters because hooks invoke it on every session and commit; trivial cross-compilation |
| Distribution | `curl \| sh` + Homebrew tap + `@ilk/cli` npm wrapper | npm's `ilk` name is taken by a dormant JS library (`0.3.1`); PyPI `ilk` is free. Binary stays `ilk` everywhere |
| Manifest format | YAML (`ilk.yaml`, `layer.yaml`) | Same format as the markdown frontmatter layer authors already write; one format to learn |
| Templating | Go `text/template` + sprig | Familiar from Helm; no new DSL |
| Fence syntax | `<!-- ilk:begin layer=x -->` / per-filetype comment equivalents | Article's fence rule; comment style resolved by file extension |

Alternative considered: TypeScript/Node. Better ecosystem familiarity for layer authors,
but it forces a Node runtime into Python/Rust/Go repos and adds ~100ms+ to every hook
invocation. Layers are declarative YAML + templates + shell, so the core's language barely
affects who can author one. Go wins.

## 10. Roadmap

| Milestone | Contents | Rough size |
|---|---|---|
| **M0 — Spike** | Manifest schema, file modes, fence engine, plan/apply reconcile against a fake layer. Proves adopt→edit→upgrade→drop is lossless | ~1 week |
| **M1 — Useful alone** | `init`, `check`, `brief`, `adopt/drop/plan/apply/status`, built-in `compound-memory` layer, AGENTS.md + git-hook projection, `--json` everywhere | ~2–3 weeks |
| **M2 — Extensible** | Layer SDK (`layer new/test`), capabilities resolution, subcommand dispatch, git/`gh:` sources, lockfile + digests, `doctor` | ~2 weeks |
| **M3 — Ecosystem** | Registry index + `search`/`publish`, adapters for Cursor/Codex/Copilot/Gemini, second wave of first-party layers (`quality-gates`, `ralph`, `llm-wiki`, `adr`, `pr-prep`), instruction budget lint | ~3 weeks |
| **M4 — Hardening** | Three-way upgrade merges, `--pure` / exec consent, provenance verification, docs site | ~2 weeks |

M1 is the real proof point: does a repo with only `ilk init` run feel better to work in
than the same repo without it? If the answer is no, the layer system is decoration.

## 11. Risks and open questions

**Risks**

- *Upgrade merge UX* is the hardest engineering problem here and the one most likely to
  produce distrust. Anything short of "never silently destroys my edits" kills adoption.
- *Instruction bloat.* Every adopted layer wants AGENTS.md space. Without the budget lint,
  a ten-layer repo has a worse agent experience than a zero-layer one.
- *Scope creep into project management.* Linear sync, status dashboards, ticket graphs.
  Keep these as layers or the core becomes unmaintainable and opinionated.
- *Ecosystem cold start.* The registry is worthless until it has ~10 layers people want.
  First-party layers have to be genuinely good, not demos.
- *Testing matrix.* "Works with any agent" is a claim that decays silently as agents change
  their config formats. Needs adapter conformance fixtures in CI.

**Open questions for you**

1. **Where does the record's *schema* live** — hardcoded in the `compound-memory` layer, or
   a separate schema mechanism layers can extend? Affects how deep validation can go.
2. **Should `ilk` ship an MCP server** so agents get structured tools rather than shelling
   out? I'd defer to post-M3; the CLI covers it and MCP narrows portability.
3. **Multi-repo / org-level layers.** Tenex's value comes partly from 50+ skills shared
   across projects. Is an org-level `ilk` config (a private registry + pinned profile) in
   scope, or a later product?
4. **How opinionated should `init` be?** §6 proposes four folders with configurable names.
   The alternative is folder-agnostic — ilk learns your existing layout. More work, wider
   applicability, weaker defaults.
5. **Name.** npm's `ilk` is taken (dormant). Fine for a binary + tap + scoped npm package,
   but worth confirming you're not attached to `npm i -g ilk` working literally.
