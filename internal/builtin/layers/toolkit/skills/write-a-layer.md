# Write a layer

Turn a practice worth repeating into something installable, upgradeable and removable.

## When

A convention in this repository should travel — to another project, or to whoever
adopts it next. Or you are changing a layer this repository already has.

## Scaffold, then test

```
ilk layer new my-pattern --namespace you
ilk layer validate layers/my-pattern
ilk layer test layers/my-pattern
ilk adopt ./layers/my-pattern
```

`ilk layer test` is the one that matters. It adopts the layer into a throwaway
repository containing files a human wrote, applies twice to prove idempotency, drops
it, and asserts the repository came back. A layer that fails this cannot be safely
tried, and a layer nobody can safely try is one nobody will adopt.

## The decision that matters most: ownership mode

Before adding any file, decide who owns it.

| Mode | Use when |
|---|---|
| `managed` | The file exists only because your layer does. Upgrades overwrite it; drop deletes it. |
| `region` | The file belongs to the repository and you are a guest in it — `AGENTS.md`, `.gitignore`, CI config. |
| `create-only` | You are giving somebody a starting point they will edit. Never touched again, never removed. |
| `append-once` | One idempotent addition, keyed by a marker. |

**Prefer `region` for anything a human might also write in.** A layer that claims
`AGENTS.md` outright will fight every other layer and every person who opens it. Two
layers can each own a region in one file; two layers cannot each own the whole file,
and ilk refuses that rather than letting the last one win.

If you find yourself relying on the three-way merge to protect user edits inside a
`managed` file, the mode is wrong.

## Instructions versus skills

This is the second decision, and it decides whether your layer is a good citizen.

- **`instructions:`** are in every context window, for ever. Declare the token cost
  honestly — `ilk check` enforces a repository-wide ceiling, and an understated budget
  makes that ceiling a lie.
- **`skills:`** cost nothing until their situation applies. Put the detail here.

Write a skill's `description` as *the situation that should make an agent open the
file*, not as a summary of its contents. That sentence is what the decision is made on.

If you are writing a procedure in `instructions:`, it belongs in a skill.

## Checks must carry their fix

```yaml
checks:
  - id: my-pattern.coverage
    title: Coverage has not regressed
    requires: test.command      # skipped, not failed, when unavailable
    run: ./scripts/coverage.sh
    fix: "Add tests for the uncovered lines listed above, or lower the threshold in .coveragerc if the drop is intentional and explained."
```

The manifest will not validate without `fix`. Write it as an instruction to somebody
who has the failure in front of them and no other context — the caller is often an
agent that can repair the problem itself if told precisely enough what it is.

Built-in kinds that need no script: `builtin.frontmatter`, `builtin.naming`,
`builtin.links`, `builtin.references`, `builtin.section`, `builtin.stale`,
`builtin.coverage`.

## Capabilities, not dependencies

```yaml
requires: [test.command]
provides: [gates.tests]
```

Never depend on another layer by name. Declare the capability you need; anything that
supplies it satisfies you. This is what lets your layer compose with layers you have
never seen.

## Staying easy to say yes to

A layer that only renders files needs no consent to adopt, because `ilk plan` shows
every byte first. One that ships scripts, command-based checks or subcommands requires
`--allow-exec` from whoever adopts it. Staying declarative is worth real effort.

## Publishing

Push the directory to a git repository. That is the whole process:

```
ilk adopt gh:you/my-pattern
ilk adopt gh:you/monorepo/layers/my-pattern@v1.2.0
```

Run `ilk layer test` in your own CI, so adopters inherit the guarantee rather than
your assurance.

## Versioning

Breaking: removing or renaming a check id (somebody's hook references it), removing a
command, changing a `managed` file's `dest` (the old one is orphaned), adding a
`requires:` (existing adopters start failing to satisfy it).

Minor: adding a check, a skill or an instruction. Growing your instruction budget is
minor too, and worth thinking twice about.

**Small, localised changes merge; wholesale rewrites collide.** Restructuring an
instruction block people commonly edit will conflict for every one of them. Save that
for a major version, where the disruption is expected.
