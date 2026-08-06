# Configure ilk

Change what ilk does in this repository.

## When

A check is skipped for want of a capability; you are adding support for another coding
agent; a layer's defaults do not fit; or the instruction budget needs deciding.

## Everything lives in `.ilk/config.yaml`

It is the desired state, and editing it by hand then running `ilk apply` is a
first-class way to work — the commands below are conveniences over the same file.

```yaml
version: 1
targets:
  - claude-code
capabilities:
  test.command: go test ./...
  lint.command: golangci-lint run
layers:
  - id: ilk/record
    version: 0.1.0
    vars:
      docs_dir: docs
budget:
  instructions: 1500
```

Comments you add are preserved when ilk rewrites the file.

## Capabilities

A capability is how a layer asks "how does this project verify itself" without caring
what language it is written in. `quality-gates` needs `test.command`; anything that
supplies it satisfies the requirement.

```
ilk adopt quality-gates --set test.command="pytest -q"
```

or add it to `capabilities:` directly and `ilk apply`.

A check that needs a capability nobody supplies **skips** rather than fails, and says
so. If `ilk check` shows a skipped check you wanted, this is why.

Conventional names: `test.command`, `lint.command`, `build.command`, `format.command`.
Use those before inventing new ones — a layer you adopt later will look for them.

## Agent targets

```
ilk agents list                 # what is configured, and what is available
ilk agents add cursor
ilk agents remove copilot
ilk agents sync                 # regenerate every projection
```

Adding a target only changes generated files. Nothing a layer provides depends on it:
`AGENTS.md` and git hooks are always on, and every generated hook and command just
calls `ilk`. An agent with no integration at all still gets the full feature set by
running the same commands a human would.

Use `ilk doctor` afterwards to see which lifecycle events that agent can actually
deliver. Some can deliver none, and that is worth knowing rather than assuming.

## Layer variables

```
ilk info record                 # shows each variable, its default and meaning
```

Set them per layer under `vars:` in `.ilk/config.yaml`, then `ilk apply`. Changing a
variable that renames a directory does **not** move existing files — ilk will create
the new directory and leave the old one, because moving your content is not its
decision. Move it yourself, then apply.

## The instruction budget

```yaml
budget:
  instructions: 1500
```

The ceiling, in tokens, on always-on agent instructions across all layers. It exists
because unbounded instruction files measurably make agents worse — published
evaluations found lower task success and higher cost, mostly from restating what the
repository already showed.

When `ilk.budget` fails, the honest fix is almost never raising the number. Move detail
out of a layer's `instructions:` and into a skill that loads when its situation
applies. Raise the ceiling only if you have measured that the extra context earns its
place.

## Check exemptions

```
ilk baseline list                        # files exempt from a layer's checks, and why
ilk baseline clear docs/old-guide.md     # conform it, then hold it to the rules
```

Files that were already present when a layer arrived are exempt from that layer's
checks — a layer governs what happens next, not what came before. The exemption is a
ratchet: clear entries as you conform them. It is not meant to be permanent.

## What not to do

- Do not add a capability pointing at a command that does not work. A gate that always
  passes is worse than no gate, and nobody will notice for months.
- Do not raise the instruction budget to silence `ilk.budget` without reading what is
  consuming it. `ilk check` names the per-layer breakdown.
- Do not edit `.ilk/lock.json` by hand. It is ilk's record of what it wrote; changing
  it makes ilk believe things about the repository that are not true.
