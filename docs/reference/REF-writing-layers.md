---
id: ref-writing-layers
title: Writing a layer
status: current
updated: 2026-08-06
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
  kind: process               # record | process | gate | harness | integration | target
ilk: ">=0.1.0"
```

Facets are advisory tags for discovery, not a taxonomy you have to fit into. A layer
that is memory *and* execution *and* verification at once is normal; pick whichever
facet helps somebody find it.

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

`fix` is mandatory, and the manifest will not validate without it. **A check that can
only say "invalid" is a bug in the check.** The caller is often an agent that can
repair the problem itself if told precisely enough what it is — that is the entire
reason checks exist rather than documentation saying "please remember to".

Write the fix as an instruction to somebody who has the failure in front of them and
no other context.

Built-in kinds: `builtin.frontmatter`, `builtin.naming`, `builtin.links`,
`builtin.stale`, `builtin.coverage`, `builtin.drift`, `builtin.budget`,
`builtin.conflicts`.

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

Events: `session-start`, `pre-commit`, `pre-push`, `post-edit`, `pre-tool-use`.

Declare the event, not the mechanism. ilk routes it to git hooks and to whichever
configured agents can deliver it, and `ilk doctor` reports where nothing can.

`blocking: true` means a non-zero exit stops the commit or push. Use it for gates that
must hold, and leave it off for anything advisory — a blocking hook that fires
spuriously teaches people to pass `--no-verify` by reflex, which costs you every gate.

## Commands

```yaml
commands:
  - name: report
    summary: "Print the coverage report: ilk my-pattern report [--format json]"
    run: ./scripts/report.sh
```

Available as `ilk my-pattern report`. Arguments are passed through, shell-quoted. The
script runs from the repository root with `ILK_LAYER`, `ILK_REPO_ROOT` and
`ILK_VAR_<NAME>` in the environment.

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
