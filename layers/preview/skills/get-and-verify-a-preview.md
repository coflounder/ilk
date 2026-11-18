# Get and verify a preview

Look at a pull request's change running, without making the preview less safe than
it was before you looked.

## When

You are asked to check a change on its preview, a reviewer wants a link, `ilk check`
reports a `preview.` check, or you are about to reach for `wrangler`, a deploy or a
token to see a branch running.

## What a preview is here

One preview per pull request, built from the branch by CI — Cloudflare Workers Builds
or the repository's pipeline — as a Worker **version**: uploaded with
`wrangler versions upload --preview-alias <branch>`, never deployed. Production keeps
serving what it served until a person merges. Every preview sits behind a Cloudflare
Access application of its own, and the Worker verifies Access's token itself.

## Procedure

1. **Let CI build it.** Push the branch and wait for the pull request's build. Do not
   upload from your machine: a version built from a working tree is a preview of
   something nobody can name afterwards. `ilk preview url --upload` refuses outside CI
   for this reason. Never run `wrangler deploy`, `wrangler versions deploy` or
   `wrangler rollback` — each one changes production.

2. **Get the URL.**

   ```sh
   ilk preview url              # the current branch
   ilk preview url feat/x --json
   ```

   This works the alias URL out locally and calls nothing. If it says
   `preview.workers_subdomain` is not set, ask for the account's workers.dev
   subdomain (it is not a secret) rather than guessing it.

3. **Open it with the service token, and only that way.**

   ```sh
   ilk preview open "$(ilk preview url)"
   ilk preview open "$(ilk preview url)/api/health" --body
   ```

   The token's id and secret come from the environment variables the layer names.
   If they are unset, say so and stop: do not look for the secret in files, the
   dashboard, another integration or a chat, and never ask anyone to paste it. The
   command sends the token only to this Worker's own preview hosts, only over https,
   and never follows a redirect.

4. **Read, never write.** A preview runs with the Worker's own bindings, which are
   production's unless previews were given data of their own (`preview.separate-data`
   says which). A write in a preview is a write to production. `ilk preview open`
   only GETs; do not work around that with curl, a browser session or a script. If
   the change needs a write to verify, say so and hand it to a person.

5. **Say what you saw.** The status, and what the page or endpoint answered, against
   what the change was meant to do. A 200 from the shell of a single-page app proves
   only that the HTML loaded — open the endpoint the change touched.

## When it does not answer

| What you see | What it means |
|---|---|
| Redirect to `*.cloudflareaccess.com` | Access did not accept the token: the preview's Access application needs a Service Auth policy including it, or the token expired. A person fixes that in Zero Trust. |
| 401 or 403 from the Worker | Access let you in, but the Worker refused: its verification does not accept the preview application's audience (`preview.preview-audience`), or the path is write-only. |
| Error 1042 | No preview exists at that address. A Worker that implements a Durable Object gets no preview URL (`preview.no-durable-objects`). |
| The build failed | A change that adds, renames or deletes a Durable Object class cannot be uploaded as a version at all; it ships with the production deploy. Say so rather than retrying. |

Do not turn on `workers_dev`, remove the Access check, or loosen a policy to get a
preview to answer. Each of those is the thing the checks exist to catch.

## Teardown

There is nothing to tear down. A version is not deployed, so it serves nothing on
production's routes; the branch's alias keeps pointing at its last version and is
reused if the branch is pushed again. Wrangler offers no command to delete a version
or an alias at the time of writing, and this layer offers none. Closing the pull
request is the end of the preview's useful life.
