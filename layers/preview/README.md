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
- **Previews** as Worker *versions*. A pull request's branch is uploaded with
  `wrangler versions upload --preview-alias <branch>` by Cloudflare Workers Builds (the
  non-production branch build command) or by CI — never from a laptop, and never
  deployed. `preview_urls: true` gives each version a URL, and a *separate* Access
  application covers those URLs, with a policy for people and a Service Auth policy for
  agents.
- **The Worker verifies Access itself.** It reads `cf-access-jwt-assertion`, checks the
  signature against the team's keys, the issuer, and that the audience is production's
  or the preview application's AUD tag (both in `vars`; AUD tags are not secrets).
- **Agents open previews with an Access service token**, read from environment
  variables, and the Worker lets a service token (which carries `common_name` and no
  `email`) read but never write.

## Commands

| Command | Does |
|---|---|
| `ilk preview url [branch]` | Prints the branch's preview alias URL, worked out locally from the branch, the Worker's name and `workers_subdomain`. Calls nothing. `--json` gives `{url, alias, provider, worker, branch, uploaded}`. |
| `ilk preview url --upload` | In CI only (`CI` or `WORKERS_CI` set): runs `wrangler versions upload --preview-alias <alias>` and prints the URL Wrangler reports. The only Wrangler call in the layer; nothing here deploys. |
| `ilk preview open <url>` | One GET through Access with the service token from `CF_ACCESS_CLIENT_ID` / `CF_ACCESS_CLIENT_SECRET` (the names are settings). Reports the status; `--body` prints the response. Fails plainly when the variables are unset. |

`ilk preview open` keeps the token tight:

- the secret reaches curl on standard input, never on a command line, and is never
  printed or written to a file;
- it is sent only over https, only to this Worker's own preview hosts
  (`<prefix>-<worker>.<subdomain>.workers.dev`) or a host listed in `token_hosts` —
  "anything on workers.dev" would hand it to any Worker anybody can create;
- redirects are reported, never followed, because a followed redirect would carry the
  token's headers with it.

The alias is the branch lowercased, with everything but letters and digits turned to
dashes, prefixed `br-` if it would not start with a letter, and cut so
`<alias>-<worker>` fits one 63-character DNS label. For `ilk preview url` to name the
URL CI produced, CI must upload with that alias: run `ilk preview url --upload` as the
non-production build command, or pass `--preview-alias "$(ilk preview url --json | jq
-r .alias)"` to Wrangler yourself.

## Checks

| Check | Holds |
|---|---|
| `preview.workers-dev-off` | `workers_dev` is explicitly `false`. Wrangler turns it on when absent. |
| `preview.urls-on` | `preview_urls` is explicitly `true`, so uploaded versions have URLs. |
| `preview.access-verified` | Non-test Worker source reads `cf-access-jwt-assertion` and verifies it (`jwtVerify`, `crypto.subtle.verify` or the team's `/cdn-cgi/access/certs`). |
| `preview.preview-audience` | `vars` has the preview Access application's AUD (`ACCESS_AUD_PREVIEW` by default) and the Worker's source reads it. |
| `preview.no-durable-objects` | The Worker implements no Durable Object. Cloudflare makes no preview URL for one that does; a cross-script binding (`script_name`) is fine. |
| `preview.separate-data` | No KV, D1, R2, Durable Object, queue, Hyperdrive, Vectorize, workflow or service binding is shared with production — or `shared_bindings: read-only` accepts it on purpose. |
| `preview.no-committed-token` | No tracked file assigns a value to the service token's secret, and Wrangler `vars` do not carry it. |

All of them read files only: no check calls Cloudflare, runs Wrangler or needs a
credential. `preview.workers-dev-off` and `preview.no-committed-token` run on a blocking
`pre-commit` hook, because neither is ever legitimately red.

ilk checks have no warning level, so the two that describe a trade-off rather than a
mistake are separate checks with their own fix:

- `preview.no-durable-objects` fails for as long as previews cannot exist. That is the
  truth about the repository, and a red check is more honest than a pipeline that
  uploads versions nobody can open. `ilk check --skip preview.no-durable-objects` while
  the class is being moved out.
- `preview.separate-data` fails until previews get data of their own, or somebody runs
  `ilk config set preview.shared_bindings read-only` having confirmed the Worker refuses
  every write from a service token.

`preview.no-committed-token` knows one thing a general scanner does not: the name of
the variable this layer reads the secret from. It is not a secret scanner. Pair it with
[`secrets`](../secrets/) for every other credential, and for what to do once one leaks.

## Teardown

There is nothing to tear down. A version is not deployed, so it serves nothing on
production's routes; a branch's alias points at its latest version and is reused when
the branch is pushed again. Wrangler offers no command to delete a version or an alias
at the time of writing, so the layer offers none rather than a command that pretends.

## What a preview shares with production

Everything bound at the top level. Workers Builds and `wrangler versions upload` upload
a version of *the same Worker*, with the same bindings, so a write in a preview is a
write to production's KV, D1, R2 or Durable Objects. KV's `preview_id`, D1's
`preview_database_id` and R2's `preview_bucket_name` are for `wrangler dev`, not for
versions, and do not separate them. Separate data means a separate preview Worker with
bindings of its own.

## Variables

| Variable | Default | Meaning |
|---|---|---|
| `provider` | `cloudflare` | Where previews are built. Only `cloudflare` for now. |
| `wrangler_config` | `auto` | The Wrangler file; `auto` takes wrangler.jsonc, wrangler.json, then wrangler.toml. |
| `worker_name` | `auto` | The Worker; `auto` reads `name`. |
| `worker_src` | `auto` | Where the Worker's source is; `auto` is the directory of `main`. |
| `workers_subdomain` | *(empty)* | The account's workers.dev subdomain. Needed by `url` without `--upload`, and by `open`. Not a secret. |
| `wrangler` | `npx wrangler` | How `--upload` runs Wrangler. |
| `client_id_env` | `CF_ACCESS_CLIENT_ID` | The variable holding the service token's id. |
| `client_secret_env` | `CF_ACCESS_CLIENT_SECRET` | The variable holding its secret. |
| `token_hosts` | *(empty)* | Extra hosts the token may go to, e.g. the production domain. |
| `preview_audience_var` | `ACCESS_AUD_PREVIEW` | The `vars` entry with the preview Access application's AUD. |
| `shared_bindings` | `refuse` | `read-only` accepts previews sharing production's bindings. |

## What it does not do

- Read Workers Builds' settings. Whether the non-production build runs
  `wrangler versions upload` is configured in the dashboard, where no file shows it.
- Read `env.<name>` blocks. The checks read the top level of the Wrangler file.
- Parse every TOML file. Tables, arrays of tables, `key = value` and inline arrays of
  flat inline tables are read; anything subtler is skipped.

## Remove

```sh
ilk rm preview
```

The scripts and the AGENTS.md section go. Nothing in the repository was seeded.
