---
id: ref-writing-layers
title: Writing a layer
status: current
updated: 2026-11-18
covers:
  - internal/manifest/**
  - internal/builtin/layers/**
---

# Writing a layer

A layer is a directory with a `layer.yaml`. Publishing one should cost about as much
as publishing the post that described the idea.

```sh
ilk layer new my-pattern --namespace you
ilk layer validate layers/my-pattern
ilk layer test layers/my-pattern
ilk add ./layers/my-pattern
```

Push the directory to a git repository and anyone can add it:

```sh
ilk add gh:you/my-pattern
ilk add gh:you/monorepo/layers/my-pattern@v1.2.0
```

There is no registration step and no account.

---

## The manifest

```yaml
id: you/my-pattern            # namespace/name, lowercase
version: 0.1.0                # semver
summary: One line. This is what `ilk search` and `ilk list` show.
facets:
  arc: quality                # context | planning | execution | quality | release | operations
  kind: process               # record | process | gate | harness | integration
for: [ui]                     # any | ui | service | library | infra | unattended-agents | multi-agent
ilk: ">=0.1.0"
examples:                     # the layer in use, shown by `ilk info`
  - ilk my-pattern report
  - ilk check --only my-pattern
```

`examples:` are what `ilk info` leads with, so somebody deciding whether to add the
layer sees it being used before they read what it contains. One command line each.

Facets and `for:` are for discovery, and each draws from a closed list — a value
outside it is a manifest error, because free text drifts and a word nobody else uses
finds nobody. A layer that is memory *and* execution *and* verification at once is
normal; pick whichever `arc` and `kind` help somebody find it.

`for:` says which repositories the layer is meant for, by trait:

| Trait | The repository |
|---|---|
| `any` | every repository, whatever it builds — and then it stands alone |
| `ui` | has a user interface people look at |
| `service` | runs somewhere and serves requests |
| `library` | is consumed by other code through its API |
| `infra` | declares infrastructure as code |
| `unattended-agents` | has agents working with nobody watching |
| `multi-agent` | has several agents or harnesses working at once |

`ilk search --for ui` lists the layers that name a trait, `ilk info` shows them, and
`ilk init` detects the simple ones (`ui`, `service`, `library`, `infra`) from the files
in a repository and suggests the layers declared for them. A layer for one stack names
that trait rather than `any`, so it is suggested where it fits and not everywhere. The
list lives in `internal/manifest/vocab.go`; a new trait is a change to ilk, made once,
for everyone.

## Capabilities, not dependencies

```yaml
requires: [test.command]
provides: [gates.tests]
```

A layer never depends on another layer by name. It declares the *capability* it needs,
and anything that supplies it — the repository's `.ilk/config.yaml`, or another
layer already present — satisfies the requirement.

This is what lets a gate layer work in a Rust repo and a Python repo without knowing
either exists, and what lets your layer compose with layers you have never seen.

Conventional capabilities: `test.command`, `lint.command`, `build.command`,
`format.command`, `record.docs`, `record.log`.

An unmet requirement is a warning, not a failure. The layer still lands; checks that
need the capability skip with a message saying how to supply it.

### One layer per job

Every capability has one provider. Two layers that provide the same capability, or
that ship the same skill, check id, MCP server, hook name or short name, clash, and
`ilk plan` refuses them. `ilk add` names the layer already doing the job and offers
`--replace`, which swaps it out in the same plan.

Some jobs are roles that ilk names, so layers written by different people use the same
word for them: `role.memory`, `role.agent-loop` and `role.preview`. A layer that fills
one says `provides: [role.memory]`, and a repository can have one memory layer. Any
other `role.` name is refused, so a typo can't slip past the check.

Checks, hooks on a shared event and instructions stack: several layers can each add
their own without clashing.

## Variables

```yaml
variables:
  docs_dir:
    default: docs
    description: Where current truth lives.
  strictness:
    default: normal
    enum: [relaxed, normal, strict]
```

Defaults are the point. A layer should land cleanly with no prompts and no flags.
Templates read them as `{{ .Vars.docs_dir }}`; so do `dest:` paths and check arguments.

## Files, and who owns them

```yaml
files:
  - src: templates/config.yml.tmpl
    dest: .config/thing.yml
    mode: managed             # ilk owns it entirely
  - src: templates/README.md.tmpl
    dest: "{{ .Vars.docs_dir }}/README.md"
    mode: create-only         # seeded once, then yours
  - src: bin/helper.sh
    dest: .ilk/bin/helper.sh
    mode: managed
    exec: true
  - inline: "coverage/\n"
    dest: .gitignore
    mode: append-once
    region: ignore
  - src: templates/section.md.tmpl
    dest: CONTRIBUTING.md
    mode: region              # a fenced block inside a file somebody else owns
    region: my-pattern
    when: '{{ eq .Vars.strictness "strict" }}'
```

Choosing the mode is the most consequential decision in a layer:

| Use | When |
|---|---|
| `managed` | The file exists only because your layer does. Upgrades overwrite it; `ilk rm` deletes it. |
| `region` | The file belongs to the repository and you are a guest in it. `AGENTS.md`, `.gitignore`, CI config. |
| `create-only` | You are giving somebody a starting point they will edit. Never touched again, never removed. |
| `append-once` | One idempotent addition, keyed by a marker. |

**Prefer `region` over `managed` for any file a human might also want to write in.**
A layer that claims `AGENTS.md` outright will fight every other layer and every human
who opens it. Two layers can each own a region in one file; two layers cannot each own
the whole file, and ilk refuses that configuration rather than letting the last one win.

## Instructions and skills

```yaml
instructions:
  - id: guidance
    src: instructions/guidance.md.tmpl
    budget: 120               # your always-on token cost, declared honestly

skills:
  - name: investigate-a-flake
    description: Diagnose a test that fails intermittently. Use when a test passes on re-run, or CI is red on a commit that is green locally.
    src: skills/investigate-a-flake.md
```

The split matters more than anything else about a layer's quality:

- **Instructions** are in every context window, forever. Unbounded instruction files
  measurably make agents worse — published evaluations found lower task success and
  higher cost, mostly from restating what the repository already showed. Budget is
  enforced: `ilk check` fails when a repository's total crosses its ceiling.
- **Skills** cost nothing until their situation applies. Put the detail here.

Write the `description` as *the situation that should make an agent open the file*, not
as a summary of the contents. That sentence is what the decision is made on.

A good rule: if you find yourself writing a procedure in `instructions`, it belongs in
a skill. If you find yourself writing more than a paragraph of background in a skill,
it probably belongs in the repository's own documentation.

## Checks

```yaml
checks:
  - id: my-pattern.coverage
    title: Coverage has not regressed
    requires: test.command          # skipped, not failed, when unavailable
    run: ./scripts/coverage.sh
    fix: "Add tests for the uncovered lines listed above, or lower the threshold in .coveragerc if the drop is intentional and explained."
  - id: my-pattern.naming
    kind: builtin.naming            # implemented in ilk, no script needed
    fix: "Rename the file to match the grammar."
    args:
      rules:
        - dir: "{{ .Vars.docs_dir }}"
          pattern: "^[A-Z]{2,6}-[a-z0-9-]+\\.md$"
          example: ARCH-overview.md
```

Check ids start with the layer's short name and a dot: `my-pattern.coverage` in
`you/my-pattern`. `ilk layer validate`, and every load of a layer, refuse any other
id. The prefix keeps ids from colliding across layers, and lets
`ilk check --only my-pattern` select the whole layer.

A `run:` check that needs a credential names the variable that carries it with
`requires_env: [PULUMI_ACCESS_TOKEN]`. ilk tests that it is set, never reads it, and
skips the check with that reason while it is absent, so a missing token does not read
as a broken repository. An entry may be a template — `requires_env: ["{{ .Vars.credentials }}"]`
— when which variable carries the credential is the adopter's choice; one that renders
empty requires nothing. `infra` uses this, because Pulumi Cloud, a self-managed Pulumi
backend and Terraform each read a different one.

`fix` is mandatory, and the manifest will not validate without it. **A check that can
only say "invalid" is a bug in the check.** The caller is often an agent that can
repair the problem itself if told precisely enough what it is — that is the entire
reason checks exist rather than documentation saying "please remember to".

Write the fix as an instruction to somebody who has the failure in front of them and
no other context.

Built-in kinds: `builtin.frontmatter`, `builtin.naming`, `builtin.links`,
`builtin.stale`, `builtin.coverage`, `builtin.drift`, `builtin.budget`,
`builtin.conflicts`, `builtin.references`, `builtin.section`, `builtin.limit`,
`builtin.forbid`.

### Script checks: the exit contract and findings

A `run:` check's exit code decides its result:

| exit | result | meaning |
|------|--------|---------|
| 0 | pass | the repository is right |
| 1 | fail | the repository is wrong; the fix applies |
| anything else | error | the check itself could not run — a missing script (127), a crash, a usage mistake |

An error still fails `ilk check`, but it is reported as "could not run" rather than
as a fault in the repository, so a missing script never sends an agent off to fix
code that is fine. Write your script to exit 1 for what it finds and leave every
other code for what went wrong. When the command is somebody else's tool whose
failure codes are not 1 — a test runner, `make` — map them at the boundary, as the
gates layer does:

```yaml
run: '( {{ index .Caps "test.command" }} ) || { s=$?; [ "$s" -ge 126 ] && exit "$s"; exit 1; }'
```

A script check can report findings per file, the way builtins do, by declaring
`findings: jsonl` and printing one JSON object per line of stdout:

```yaml
checks:
  - id: my-pattern.owners
    run: .ilk/bin/owners.sh
    findings: jsonl
    fix: "Add an `owner:` to each document named above."
```

```sh
printf '{"path":"%s","line":%d,"message":"%s"}\n' "$file" 1 "no owner:"
exit 1
```

`message` is required; `path` and `line` are optional. They appear in `ilk check`
and its `--json` output exactly as a builtin's findings do. A line that is not such
an object is skipped with a warning naming the line, so a stray `echo` costs a
warning, not the check. Stderr is kept as the check's output. The exit code still
decides pass or fail: findings from a script that exits 0 are reported with a
warning, and a script that exits 1 without printing any gets a generic finding.

### A note on `builtin.stale`

If your layer governs documents, resist the temptation to expire them on a timer.
`builtin.stale` measures a document against the paths it declares in `covers:`, in
commits rather than days, because no single expiry suits projects of different
maturity — and because a timer trains people to bump a date instead of reading the
document. Pair it with `builtin.coverage`, which reports documents that declare
nothing to be measured against, and patterns that match nothing.

## Hooks

```yaml
hooks:
  - event: pre-commit
    blocking: true
    run: ilk check --only my-pattern.naming
```

Events: `session-start`, `turn-end`, `pre-commit`, `pre-push`, `post-edit`, `pre-tool-use`.

`turn-end` maps to Claude Code and Codex `Stop` events, and to Pi's `agent_settled`.
Adapters identify themselves with `ilk hook run <event> --target <name>`, which
sets `ILK_HARNESS` for the layer command. Native hook JSON input is replayed to
all hooks registered for the event. A blocking `turn-end` failure exits 2 and its
output becomes continuation feedback. Hooks must bound their own retries using the
native `stop_hook_active` input; a publication request must not trap the agent in a
stop loop. Git events retain exit 1 for failure and do not consume stdin.

Declare the event, not the mechanism. ilk routes it to git hooks and to whichever
configured agents can deliver it, and `ilk doctor` reports where nothing can.

`blocking: true` means a non-zero exit stops the commit or push. Use it for gates that
must hold, and leave it off for anything advisory — a blocking hook that fires
spuriously teaches people to pass `--no-verify` by reflex, which costs you every gate.

## MCP servers

```yaml
mcp:
  - name: linear
    summary: Linear's MCP server, over the stdio bridge.
    command: npx
    args: ["-y", "mcp-remote", "https://mcp.linear.app/sse"]
    requires_env: [LINEAR_API_KEY]
```

Declare the server, not the agent's config file. Each configured target renders its own
projection — `.mcp.json` for Claude Code, `.cursor/mcp.json` for Cursor,
`.codex/config.toml` for Codex, or a project extension for Pi — and every
entry ilk writes says the same thing: `ilk mcp run <name>`. The real command stays in
the manifest and is resolved when the agent starts the server, so changing `command:`
or `args:` never rewrites agent config. JSON/TOML configuration is co-owned;
Pi's adapter owns only its extension directory. User configuration is preserved on
add and remove. Codex refuses a name collision with a user-owned MCP server.

`args:` and `env:` values are templated over the layer's variables and the repository's
capabilities, like every other manifest value.

Credentials never go in the manifest, and never end up in a committed file.
`requires_env:` names the variables the server needs; `ilk mcp run` tests them for
presence — without reading them — and refuses to start with a message naming what is
missing, instead of the agent reporting an opaque connection failure. `ilk mcp list`
shows the same thing at a glance.

Server names are a repository-wide namespace: two adopted layers declaring the same
name refuse to plan, rather than one silently winning.

## Commands

```yaml
commands:
  - name: report
    summary: Print the coverage report
    args:
      - {name: package, required: true, usage: the package to report on}
      - {name: more, variadic: true, usage: further packages}
    flags:
      - {name: format, usage: "text or html"}
      - {name: open, short: o, type: bool, usage: open the report when it is written}
      - {name: threshold, type: int, default: "80", usage: the coverage to report against}
    examples:
      - ilk my-pattern report ./internal/payments
      - ilk my-pattern report ./internal/payments --format html -o
    json: true          # the script honours ILK_OUTPUT=json
    run: ./scripts/report.sh
```

Available as `ilk my-pattern report`. ilk builds a real command from the
declaration, so the layer gets what built-in commands have: a usage line made from
`args`, `--help` with the flags and examples, shell completion, and usage errors
(exit 2) for a missing argument, an extra one, an unknown flag or an `int` flag that
is not a number — all before the script runs.

What the script receives:

- **Arguments** as positional parameters, shell-quoted, after anything in `run:`.
  `required: true` arguments come first; one `variadic: true` argument may come
  last and takes the rest. An argument that starts with `-` goes after `--`.
- **Flags** as `ILK_FLAG_<NAME>`, upper-cased with dashes as underscores. A `bool`
  is `1` or `0`. A `string` or `int` is the value given, or `default:` when it was
  not given, or empty. Every declared flag is set, given or not, so
  `${ILK_FLAG_FORMAT:-text}` is all a script needs. `type:` is `bool`, `string`
  (the default) or `int`; `short:` is one letter.
- **ilk's own flags never.** `--json` becomes `ILK_OUTPUT=json` and `-C` chooses the
  repository the script runs in. A layer cannot declare `--json`, `--dir`, `--help`,
  `-C` or `-h`.
- `ILK_LAYER`, `ILK_REPO_ROOT`, `ILK_OUTPUT` and `ILK_VAR_<NAME>`. The script runs
  from the repository root.

`json: true` says the script prints JSON when `ILK_OUTPUT=json`. A command without it
refuses `--json` with exit 2 and the error code `json_unsupported`, rather than
answering a caller who asked for JSON with text. Declare it only when the script
honours it.

The script's own exit code is passed through unchanged.

Built-in verbs are never shadowed: a layer named `check` cannot take over `ilk check`.

## Testing

```sh
ilk layer test ./layers/my-pattern
```

Adds the layer to a throwaway repository containing files a human wrote, applies
twice to prove idempotency, removes it, and asserts the repository came back:

```
you/my-pattern 0.1.0

  ✓ 8 artifact(s)
  ✓ add is idempotent
  ✓ rm restores the repository
  · handed over: docs/reference/README.md
```

Layers that fail this cannot be safely tried, and a layer nobody can safely try is one
nobody will add. Run it in your own CI.

## Trust

Adding a layer from a source ilk did not ship requires explicit consent when that
layer ships executable content — scripts, command-based checks, or subcommands:

```sh
ilk info gh:someone/theirs      # read it first
ilk add gh:someone/theirs --allow-exec
```

Declarative layers — files, templates, built-in checks, hooks that call `ilk` — need
no consent, because `ilk plan` shows every byte before anything is written.

If you are publishing a layer, staying declarative makes it dramatically easier for
somebody to say yes to.

## What upgrades do to your users

When you ship a new version, ilk does not overwrite files people have edited. It
merges: where their changes and yours touch different parts of a file, both survive;
where they collide, ilk refuses and tells the user, naming the lines.

Two consequences for you as an author:

- **Small, localised changes merge; wholesale rewrites collide.** Restructuring an
  instruction block that people commonly edit will conflict for every one of them.
  Prefer additive changes, and save the rewrite for a major version where the
  disruption is expected.
- **A file people are meant to edit should be `create-only`, not `managed`.** If you
  find yourself relying on the merge to protect user edits in a `managed` file, the
  mode is wrong.

## Versioning

Layers are semver. Treat these as breaking:

- removing or renaming a check id (somebody's hook references it)
- removing or renaming a command
- changing a `dest` path for a `managed` file (the old one is orphaned)
- adding a `requires:` (existing adopters start failing to satisfy it)

Adding a check, a skill, or an instruction is a minor version. Growing your instruction
budget is one too — and worth thinking twice about.
