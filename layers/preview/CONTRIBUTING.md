What this layer wants from a proposal:

  - **A second provider.** Say which cloud, how it builds one preview per pull
    request, what puts it behind sign-in, and how an agent opens it with a
    credential that can only read. A provider is a new value of `provider` plus
    the branches in the scripts that serve it, held to the same commands (`url`,
    `open`) and the same kinds of checks. If the cloud cannot meet the contract —
    no per-branch URL, no way to keep a preview private — say so; that is worth
    knowing before anybody writes the code.

  - **A check that fired on a Worker that was right.** Send the shape, not the
    configuration: which key, what form, and why it was fine. The Access check in
    particular is a grep for the header name and a verify call, and a Worker that
    verifies some other way is exactly the case it needs to learn from.

  - **A check that stayed green on a Worker that was wrong.** More urgent than
    anything else here. Send the shape with fake values.

  - **Never paste an Access service token, an account id or an AUD tag** into a
    proposal, real or redacted. Write `<preview-access-aud>`.

  - **What Cloudflare changed.** The layer encodes things Cloudflare documents and
    can change: that Workers Previews are the recommended way to preview a branch
    and Version URLs are not, that a Preview inherits no bindings or vars, that a
    Worker implementing a Durable Object gets no Version URL, the shape of preview
    hostnames, and how many Previews Cloudflare keeps. Any of those moving is a
    reason to change a check, and a link to the change is the proposal. (0.3.0
    followed Cloudflare's Workers Previews docs of September to November 2026.)

  - **The known gaps.** `env.<name>` blocks are not read, so a repository that gives
    previews their own Worker through an environment still fails
    `preview.separate-data`. Workers Builds' own settings are not visible in the
    repository, so nothing checks that Enable Preview Builds is on or that the
    Preview command runs `wrangler preview` rather than deploying. Cloudflare does
    not document how it turns a branch name into a Preview name, so `ilk preview
    url` is right only for branches already named in lowercase letters, digits
    and dashes, unless the build runs `ilk preview url --upload`. TOML is read in its common shapes only. If one of these
    cost you, say how you worked around it.

## What is tested, and how

`test/checks.yaml` asserts every check in both directions against the fixtures in
`test/fixtures/`, through `ilk layer test`. `test/run.sh` exercises the commands
against a fake `wrangler` and a fake `curl` — that `--upload` only ever calls
`wrangler preview --name` (or `versions upload` without a `previews` block) and
only in CI, that the secret never reaches a command line,
and that `open` refuses hosts that are not this Worker's. Neither can prove that
Cloudflare answers the way the fakes do, and nothing here talks to a real account.
