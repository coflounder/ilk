# ilk/infra

Infrastructure as code, with the one discipline that matters when an agent is
holding the keys: **preview is agent work, apply is human work.**

```sh
ilk info gh:coflounder/ilk/layers/infra     # read it first
ilk add gh:coflounder/ilk/layers/infra --allow-exec
```

The layer is named for that rule rather than for a tool. Pulumi's commands are the
defaults; Terraform, or anything else with a preview and an apply, is two settings
away (see [Terraform](#terraform)).

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

The layer provides both, with Pulumi's commands as defaults:

| Capability | Default |
|---|---|
| `infra.preview.command` | `pulumi preview --diff --non-interactive` |
| `infra.up.command` | `pulumi up` |

A value under `capabilities:` in `.ilk/config.yaml` wins over the default, so
choosing a stack is one setting:

```sh
ilk config set infra.preview.command "pulumi preview --diff --non-interactive --stack prod"
ilk config set infra.up.command "pulumi up --stack prod"
```

The preview command runs with a stack directory as its working directory, so it
has to select its own stack and must not prompt.

## Terraform

Change the two settings:

```sh
ilk config set infra.preview.command "terraform plan -input=false"
ilk config set infra.up.command "terraform apply"
```

A plan that runs is a clean preview, as it is for Pulumi: the gate reports whether
the change can be previewed, and the diff is for you to read. A fresh clone needs
`terraform init` in each stack first; put it in the command
(`terraform init -input=false && terraform plan -input=false`) if CI should do it
too. A Terraform stack is a
directory with `.tf` files, which the shape check accepts in place of a
`Pulumi.yaml` — delete the seeded `Pulumi.yaml` when you add yours. The preview
skips until its credential is in the environment, and that credential is
`PULUMI_ACCESS_TOKEN` unless you name another: set
`ilk config set infra.credentials AWS_ACCESS_KEY_ID` (or whatever your provider
reads), or set it to nothing if the plan needs none. `infra.no-plaintext-secrets`
reads Pulumi stack config only, so it has nothing to say about a Terraform stack;
credentials in a `.tfvars` file are the `secrets` layer's to catch.

## The shape

One stack per directory under the `infra` group — ilk's canonical name for how a
project is deployed and operated, so two unrelated layers naming it agree about
what it is. This layer is its first tenant.

```
infra/
  README.md          the group index, generated
  dns/Pulumi.yaml
  auth/Pulumi.yaml
  cms/main.tf        a Terraform stack is a stack too
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
| `infra.stack-shape` | Every directory under `infra/` has a project file — a `Pulumi.yaml` or `.tf` files. A directory without one is invisible to the preview, so the failure mode is silence. |
| `infra.no-plaintext-secrets` | No `Pulumi.yaml` or `Pulumi.*.yaml` carries a credential-shaped key with a plain value. The `secure:` form written by `pulumi config set --secret` is the only acceptable one. |
| `infra.preview` | Preview is clean for the stacks this change touched. |

The first two are wired to a blocking `pre-commit` hook. The preview is not: it
needs a network and somebody else's backend, and a blocking hook that waits on an
API teaches people to reach for `--no-verify`, which costs you every gate.

## Credentials

`infra.preview` requires the environment variable named by the `credentials`
variable, `PULUMI_ACCESS_TOKEN` by default. Without it the check **skips with a
reason** rather than running and reporting an authentication failure as a broken
repository:

```
· infra.preview  needs PULUMI_ACCESS_TOKEN in the environment — this check cannot
                tell an absent credential from a failure, so it declines to guess
```

A skip is reported by `ilk check`, never silently: the run is visibly one short
rather than green. ilk never reads the value; only its presence is tested.

A self-managed Pulumi backend authenticates with `PULUMI_CONFIG_PASSPHRASE`
instead: `ilk config set infra.credentials PULUMI_CONFIG_PASSPHRASE`.

## Variables

| Variable | Default | Meaning |
|---|---|---|
| `group` | `infra` | The canonical group infrastructure lives under. |
| `stack` | `dns` | The one stack seeded to establish the shape. |
| `runtime` | `yaml` | The runtime in the seeded `Pulumi.yaml`. |
| `credentials` | `PULUMI_ACCESS_TOKEN` | The environment variable the preview needs; it skips while that is unset. Empty means none. |
| `base_ref` | `origin/main` | What "the stacks this change touched" is measured against. A ref that does not resolve falls back to the working tree. |

## Remove

```sh
ilk rm infra
```

The scripts and the group index go. `infra/<stack>/Pulumi.yaml` was seeded once
and handed over, so it stays — as does everything you wrote after it.
