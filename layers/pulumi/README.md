# ilk/pulumi

Infrastructure as code, with the one discipline that matters when an agent is
holding the keys: **preview is agent work, apply is human work.**

```sh
ilk info gh:coflounder/ilk/layers/pulumi     # read it first
ilk add gh:coflounder/ilk/layers/pulumi --allow-exec
```

## The asymmetry

The layer declares two capabilities and treats them completely differently.

| Capability | Wired to |
|---|---|
| `infra.preview.command` | a gate in `ilk check`, so a change is previewed before anybody proposes it |
| `infra.up.command` | **nothing** |

`infra.up.command` exists so a skill can name it and a person can run it. No
check, no hook and no subcommand in this layer invokes it, and one that did would
be a bug rather than a feature. An agent that can apply infrastructure changes is
an agent that can delete a production database while reasoning about a typo — and
the mistake that gets you there is not carelessness, it is an apply that also
prints a diff and would have asked for confirmation on a terminal.

Set both in `.ilk/config.yaml`:

```yaml
capabilities:
  infra.preview.command: pulumi preview --diff --non-interactive --stack prod
  infra.up.command: pulumi up --stack prod
```

The preview command runs with a stack directory as its working directory, so it
has to select its own stack and must not prompt.

## The shape

One stack per directory under the `infra` group — ilk's canonical name for how a
project is deployed and operated, so two unrelated layers naming it agree about
what it is. This layer is its first tenant.

```
infra/
  README.md          the group index, generated
  dns/Pulumi.yaml
  auth/Pulumi.yaml
  cms/Pulumi.yaml
```

Adding the layer seeds one stack (`infra/dns` by default — set `stack` to
something your project actually deploys). Every other stack is a sibling
directory you create by hand, and the checks pick it up with no configuration.

Separate directories mean separate state and separate blast radius, which is what
lets the preview gate report on only the stacks a change touched instead of the
whole estate.

## Checks

| Check | Holds |
|---|---|
| `infra.stack-shape` | Every directory under `infra/` has a `Pulumi.yaml`. A directory without one is invisible to pulumi, so the failure mode is silence. |
| `infra.no-plaintext-secrets` | No `Pulumi.yaml` or `Pulumi.*.yaml` carries a credential-shaped key with a plain value. The `secure:` form written by `pulumi config set --secret` is the only acceptable one. |
| `infra.preview` | Preview is clean for the stacks this change touched. |

The first two are wired to a blocking `pre-commit` hook. The preview is not: it
needs a network and somebody else's backend, and a blocking hook that waits on an
API teaches people to reach for `--no-verify`, which costs you every gate.

## Credentials

`infra.preview` declares `requires_env: [PULUMI_ACCESS_TOKEN]`. Without it the
check **skips with a reason** rather than running and reporting an authentication
failure as a broken repository:

```
· infra.preview  needs PULUMI_ACCESS_TOKEN in the environment — this check cannot
                 tell an absent credential from a failure, so it declines to guess
```

A skip is reported by `ilk check`, never silently: the run is visibly one short
rather than green. ilk never reads the value; only its presence is tested.

A self-managed backend authenticates with `PULUMI_CONFIG_PASSPHRASE` instead, and
this layer does not yet express that — see [CONTRIBUTING.md](CONTRIBUTING.md).

## Variables

| Variable | Default | Meaning |
|---|---|---|
| `group` | `infra` | The canonical group infrastructure lives under. |
| `stack` | `dns` | The one stack seeded to establish the shape. |
| `runtime` | `yaml` | The runtime in the seeded `Pulumi.yaml`. |
| `base_ref` | `origin/main` | What "the stacks this change touched" is measured against. A ref that does not resolve falls back to the working tree. |

## Remove

```sh
ilk rm pulumi
```

The scripts and the group index go. `infra/<stack>/Pulumi.yaml` was seeded once
and handed over, so it stays — as does everything you wrote after it.
