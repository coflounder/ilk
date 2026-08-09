What this layer wants from a proposal:

  - Say what the preview said and what the apply actually did. The whole premise is
    that a preview is a good enough answer for an agent and an apply is not; a case
    where the two disagreed is the most valuable thing you can send, and it is
    evidence nobody else can gather.

  - If you wired `infra.up.command` to something — a hook, a CI job, a subcommand —
    say so and say why. It is deliberately wired to nothing here, and a repository
    that needed the opposite is either a case this layer got wrong or a case for a
    separate layer. Either way we would rather hear it than have every adopter
    quietly rediscover it.

  - If `infra.no-plaintext-secrets` fired on something that was not a secret, send
    the key name and nothing else. **Never paste the value**, real or redacted. The
    check reads key names because that is the only signal a file carries, and the
    list of credential-shaped names is exactly the kind of thing that only improves
    from other people's repositories. `secretsprovider` is already excluded for
    this reason.

  - If it did *not* fire on something that was a secret, that is more urgent than
    anything else on this list. Send the shape of the entry, with a fake value.

  - **The credential story is the known gap.** `requires_env` names
    `PULUMI_ACCESS_TOKEN`, which is right for Pulumi Cloud and wrong for a
    self-managed backend, where the credential is `PULUMI_CONFIG_PASSPHRASE` and
    the backend URL. `requires_env` is a fixed list in the manifest and is not
    templated, so the layer cannot currently express "one or the other". If you run
    a self-managed backend, say how you configured around it — that is the input
    the fix needs.

  - Say whether "the stacks this change touched" was the right granularity. It is
    the union of the merge-base diff, the working tree and untracked files, which
    over-previews after a rebase and under-previews when a stack depends on another
    stack's output. A stack-dependency graph is a real thing this layer does not
    model, and a repository that needed one is the argument for adding it.

## What is deliberately untested

`infra.preview` has no case in `test/checks.yaml`, and cannot have one.

The assertion harness runs in a sandbox with no credentials, so the check skips —
and `expect:` accepts only `pass` or `fail`. Asserting it would mean either
weakening the check until it runs without a backend, or asserting something other
than what the check does. Both are worse than the gap. `ilk layer test --strict`
will name `infra.preview` as unasserted; that is correct, and it should stay named
rather than silenced.

The two file-shape checks are asserted in both directions, which is what covers
the failure the harness exists for: a check whose pattern matches nothing looks
exactly like a check that passes.
