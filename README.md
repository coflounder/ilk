# ilk

**A package manager for a repository's process, not its dependencies.**

Coding agents cannot inherit what was never written down. `ilk` manages the layers a
repository uses to work well with them — directory contracts, instructions, skills,
hooks, gates and checks — as versioned units you can adopt, upgrade and drop at any
point in a project's life.

```sh
ilk init
```

That gives you a project record, agent instructions, one validator, a session brief
and a pre-commit hook. No registry, no network, nothing to choose.

---

## Why this exists

A generator answers "what should a repo look like on day zero", then leaves. Its later
improvements never reach you, and you cannot undo what it did.

`ilk` answers day *n*. Practice is adopted incrementally, upgraded deliberately, and
removed cleanly. When someone publishes a post about a new pattern on a Tuesday, a
repository can adopt it that afternoon and drop it on Thursday when it turns out not
to fit.

Three commitments constrain everything else:

- **Any coding agent.** The CLI is the interface; agent configuration is a generated
  projection of it. Anything that can run a shell command gets the whole feature set.
- **Useful alone.** `ilk init` with no arguments produces a better repository than the
  one before it, with no network access.
- **Community-extensible.** Publishing a layer is as cheap as publishing the blog post
  that described the idea: a git repo with a manifest.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/coflounder/ilk/main/install.sh | sh
```

or

```sh
npm install -g @ilk/cli
brew install coflounder/tap/ilk
go install github.com/coflounder/ilk/cmd/ilk@latest
```

## Use

```sh
ilk init                         # set the repository up
ilk brief                        # the packet a session should start with
ilk check                        # validate; every failure prints its fix
ilk status                       # what is adopted, and what has drifted

ilk list --available             # layers you can add
ilk info quality-gates           # what a layer contains, before adopting it
ilk adopt quality-gates --set test.command="go test ./..."
ilk drop quality-gates           # removes exactly what it added

ilk adopt gh:someone/their-layer # community layers, same interface
ilk agents add cursor            # generate config for another agent
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

Layers require **capabilities**, not each other. `quality-gates` needs `test.command`;
anything that supplies it — your config, or another layer — satisfies that, so one gate
layer works in any language.

See [docs/REF-writing-layers.md](docs/REF-writing-layers.md) to write one, and
[docs/REF-design-proposal.md](docs/REF-design-proposal.md) for the reasoning behind the design.

## How drop can be safe

Every file ilk writes has an ownership mode, and the lockfile records a hash of what
ilk put there. That is the difference between "unchanged since I wrote it" (safe to
overwrite or delete) and "a human has edited this" (stop and say so).

| mode | ilk owns | on upgrade | on drop |
|---|---|---|---|
| `managed` | the whole file | overwrite | delete |
| `region` | a fenced block inside a file you own | replace the block | remove the block, keep the file |
| `create-only` | nothing after the first write | never touch | leave in place |
| `append-once` | a marked block, added idempotently | no-op | remove the block |

`region` is the load-bearing one. It means ilk can put guidance in your `AGENTS.md`,
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
$ ilk adopt quality-gates

  + create     .github/workflows/ilk.yml                    ilk/quality-gates
  + create     .git/hooks/pre-push                          target:git-hooks
  + block +    AGENTS.md [instructions]                     ilk/quality-gates
  ~ block ~    AGENTS.md [skills]                           target:agents-md

  Apply? [y/N]
```

## Working with any agent

Layers never emit agent-specific files. They declare what an agent should know and do;
targets project that into whatever each agent reads.

| Neutral artifact | Where it lands |
|---|---|
| instructions | `AGENTS.md`, plus pointer stubs for `CLAUDE.md`, `.cursor/rules/`, `.github/copilot-instructions.md`, `GEMINI.md` |
| skill | `.agent/skills/<name>/SKILL.md`, mirrored to `.claude/skills/`; indexed in `AGENTS.md` for agents with no skill support |
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
ilk layer test ./layers/mine   # prove your layer's adopt/drop round trip
```

## Licence

MIT. See [LICENSE](LICENSE).
