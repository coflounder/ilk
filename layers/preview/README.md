# ilk/preview

A preview of every pull request, behind sign-in, that an agent can find and open
read-only — and checks that the preview is not quietly public, shared with
production, or missing.

```sh
ilk info gh:coflounder/ilk/layers/preview     # read it first
ilk add gh:coflounder/ilk/layers/preview --allow-exec
ilk config set preview.workers_subdomain <name>   # the <name> in <worker>.<name>.workers.dev
ilk check --only preview
```

Or with visual-qa, which makes interface work show what it looks like:

```sh
ilk add @preview --allow-exec
```

## One layer, the cloud as a setting

The layer owns the contract — a preview per pull request, its URL as output, what
teardown means, and checks against it — and `provider` says where previews are built.
Cloudflare Workers is the first provider. Another cloud becomes another value of
`provider`, held to the same commands and the same kinds of checks, rather than a layer
of its own.

The layer `provides: [role.preview]`, so a repository has exactly one: a second preview
layer clashes with it at `ilk add`, and `--replace` swaps one for the other.

## The Cloudflare shape

What the checks hold a repository to, modelled on a Worker that runs this way in
production:

- **Production** on a custom-domain route, behind a Cloudflare Access application.
  `workers_dev: false`, so there is no public workers.dev address beside it.
- **Previews** as Workers Previews, which is what Cloudflare recommends for a branch or
  pull request. The Wrangler configuration has a `previews` block, and Cloudflare
  Workers Builds runs `npx wrangler preview` for every push to a non-production branch
  (Settings, Build, Branch control: Enable Preview Builds; Wrangler 4.135 or later in
  `package.json`). Each Preview gets its own URL at
  `<name>-<worker>.<subdomain>.workers.dev`, never from a laptop and never deployed.
  `preview_urls: true` gives the workers.dev URL, and a *separate* Access application
  covers those URLs, with a policy for people and a Service Auth policy for agents.
- **A Preview inherits nothing.** Only what the `previews` block binds reaches a
  Preview: every binding and var production has is repeated there
  (`preview.previews-complete`), stores that hold data point at preview stores
  (`preview.separate-data`), and a Durable Object class this Worker implements gets a
  namespace per Preview, deleted with it. Secrets are set with `wrangler preview
  base-config secret put` (only Previews made after it get them). Cron Triggers never
  run on a Preview.
- **Not uploaded versions.** `wrangler versions upload --preview-alias` still works, but
  Cloudflare says not to use Version URLs for branch or pull request testing: a version
  runs with production's bindings, and a Worker that implements a Durable Object gets
  no URL at all. Layer versions before 0.3.0 asked for this; `preview.workers-previews`
  now fails without a `previews` block.
- **The Worker verifies Access itself.** It reads `cf-access-jwt-assertion`, checks the
  signature against the team's keys, the issuer, and that the audience is production's
  or the preview application's AUD tag (both in `vars`; AUD tags are not secrets).
- **Agents open previews with an Access service token**, read from environment
  variables, and the Worker lets a service token (which carries `common_name` and no
  `email`) read but never write.

## Commands

| Command | Does |
|---|---|
| `ilk preview url [branch]` | Prints the branch's preview URL, worked out locally from the branch, the Worker's name and `workers_subdomain`. Calls nothing. `--json` gives `{url, alias, provider, worker, branch, uploaded}`; `alias` is the preview's name. |
| `ilk preview url --upload` | In CI only (`CI` or `WORKERS_CI` set): runs `wrangler preview --name <name>` and prints the URL Wrangler reports. With no `previews` block it runs `wrangler versions upload --preview-alias <name>` instead, and warns. The only Wrangler call in the layer; nothing here deploys. |
| `ilk preview open <url>` | One GET through Access with the service token from `CF_ACCESS_CLIENT_ID` / `CF_ACCESS_CLIENT_SECRET` (the names are settings). Reports the status; `--body` prints the response. Fails plainly when the variables are unset. |

`ilk preview open` keeps the token tight:

- the secret reaches curl on standard input, never on a command line, and is never
  printed or written to a file;
- it is sent only over https, only to this Worker's own preview hosts
  (`<prefix>-<worker>.<subdomain>.workers.dev`) or a host listed in `token_hosts` —
  "anything on workers.dev" would hand it to any Worker anybody can create;
- redirects are reported, never followed, because a followed redirect would carry the
  token's headers with it.

The name is the branch lowercased, with everything but letters and digits turned to
dashes, prefixed `br-` if it would not start with a letter, and cut so
`<name>-<worker>` fits one 63-character DNS label. Workers Builds' default Preview
command, `npx wrangler preview`, names the Preview after the branch, and Cloudflare
does not document how it treats a branch name outside that form. A branch already
named in lowercase letters, digits and dashes gets the same URL either way. For any
other branch, take the URL from the build's comment on the pull request, or make the
Preview command `npx ilk preview url --upload` (ilk installed in the build), or pass
`--name "$(ilk preview url --json | jq -r .alias)"` to `wrangler preview` yourself.

## Checks

| Check | Holds |
|---|---|
| `preview.workers-dev-off` | `workers_dev` is explicitly `false`. Wrangler turns it on when absent. |
| `preview.urls-on` | `preview_urls` is explicitly `true`, so Previews have workers.dev URLs. Left unset it follows `workers_dev`, which is off. |
| `preview.workers-previews` | The Wrangler configuration has a `previews` block, so pull requests build as Workers Previews rather than uploaded versions. |
| `preview.previews-complete` | Every binding and var at the top level is given again in the `previews` block, which is all a Preview gets. |
| `preview.access-verified` | Non-test Worker source reads `cf-access-jwt-assertion` and verifies it (`jwtVerify`, `crypto.subtle.verify` or the team's `/cdn-cgi/access/certs`). |
| `preview.preview-audience` | `vars` (and `previews.vars`, when there is a `previews` block) has the preview Access application's AUD (`ACCESS_AUD_PREVIEW` by default) and the Worker's source reads it. |
| `preview.separate-data` | No KV, D1, R2, Durable Object, queue, Hyperdrive, Vectorize, workflow or service binding is shared with production — or `shared_bindings: read-only` accepts it on purpose. |
| `preview.no-committed-token` | No tracked file assigns a value to the service token's secret, and Wrangler `vars` do not carry it. |

All of them read files only: no check calls Cloudflare, runs Wrangler or needs a
credential. `preview.workers-dev-off` and `preview.no-committed-token` run on a blocking
`pre-commit` hook, because neither is ever legitimately red.

ilk checks have no warning level, so the two that describe a trade-off rather than a
mistake are separate checks with their own fix:

- `preview.workers-previews` fails while pull requests can only be previewed as uploaded
  versions. A repository that keeps versions on purpose skips it
  (`ilk check --skip preview.workers-previews`) and says why. For a Worker that
  implements a Durable Object it also names the classes, because for that Worker
  versions get no URL at all. It replaces 0.2.0's `preview.no-durable-objects`.
- `preview.separate-data` fails until previews get data of their own, or somebody runs
  `ilk config set preview.shared_bindings read-only` having confirmed the Worker refuses
  every write from a service token.

`preview.no-committed-token` knows one thing a general scanner does not: the name of
the variable this layer reads the secret from. It is not a secret scanner. Pair it with
[`secrets`](../secrets/) for every other credential, and for what to do once one leaks.

## Teardown

Nothing needs tearing down for safety: a Preview is not deployed, so it serves nothing
on production's routes, and pushing the branch again updates the same Preview.
Cloudflare keeps 100 Previews per Worker on the Free plan and 500 on paid plans, and
deletes the one least recently deployed to when a new one would pass the limit,
together with its Durable Object data. To remove one sooner, a person runs
`npx wrangler preview delete --name <name>` or uses the Worker's Previews tab. The layer
offers no delete command: deleting is not something an agent checking a preview needs.

## What a preview shares with production

What the `previews` block binds, and nothing else. `preview.separate-data` reports a
binding there that names the same store production binds (a KV id, a database, a
bucket, a queue), one it cannot identify, and any Durable Object of another Worker. A
class this Worker implements is per Preview and is not reported.

Without a `previews` block, everything bound at the top level. An uploaded version is a
version of *the same Worker*, with the same bindings, so a write in it is a write to
production's KV, D1, R2 or Durable Objects. KV's `preview_id`, D1's
`preview_database_id` and R2's `preview_bucket_name` are for `wrangler dev`, not for
versions, and do not separate them.

## Variables

| Variable | Default | Meaning |
|---|---|---|
| `provider` | `cloudflare` | Where previews are built. Only `cloudflare` for now. |
| `wrangler_config` | `auto` | The Wrangler file; `auto` takes wrangler.jsonc, wrangler.json, then wrangler.toml. |
| `worker_name` | `auto` | The Worker; `auto` reads `name`. |
| `worker_src` | `auto` | Where the Worker's source is; `auto` is the directory of `main`. |
| `workers_subdomain` | *(empty)* | The account's workers.dev subdomain. Needed by `url` without `--upload`, and by `open`. Not a secret. |
| `wrangler` | `npx wrangler` | How `--upload` runs Wrangler (4.135 or later for `wrangler preview`). |
| `client_id_env` | `CF_ACCESS_CLIENT_ID` | The variable holding the service token's id. |
| `client_secret_env` | `CF_ACCESS_CLIENT_SECRET` | The variable holding its secret. |
| `token_hosts` | *(empty)* | Extra hosts the token may go to, e.g. the production domain. |
| `preview_audience_var` | `ACCESS_AUD_PREVIEW` | The `vars` entry with the preview Access application's AUD. |
| `shared_bindings` | `refuse` | `read-only` accepts previews sharing production's bindings. |

## What it does not do

- Read Workers Builds' settings. Whether Enable Preview Builds is on, and whether the
  Preview command runs `wrangler preview`, is configured in the dashboard, where no file
  shows it.
- Read `env.<name>` blocks. The checks read the top level of the Wrangler file.
- Parse every TOML file. Tables, arrays of tables, `key = value` and inline arrays of
  flat inline tables are read; anything subtler is skipped.

## Remove

```sh
ilk rm preview
```

The scripts and the AGENTS.md section go. Nothing in the repository was seeded.
