# Get and verify a preview

Look at a pull request's change running, without making the preview less safe than
it was before you looked.

## When

You are asked to check a change on its preview, a reviewer wants a link, `ilk check`
reports a `preview.` check, or you are about to reach for `wrangler`, a deploy or a
token to see a branch running.

## What a preview is here

One preview per pull request, built from the branch by CI — Cloudflare Workers Builds
or the repository's pipeline — as a **Workers Preview**: `wrangler preview`, configured
by the `previews` block in the Wrangler configuration, never deployed. Production keeps
serving what it served until a person merges. Every preview sits behind a Cloudflare
Access application of its own, and the Worker verifies Access's token itself.

A Preview gets only what the `previews` block gives it: its own bindings, its own vars,
and its own empty Durable Object namespace for a class this Worker implements. It runs
no Cron Triggers. A repository without a `previews` block still previews the older way,
as uploaded versions, which share production's bindings; `preview.workers-previews`
says so.

## Procedure

1. **Let CI build it.** Push the branch and wait for the pull request's build. Do not
   upload from your machine: a version built from a working tree is a preview of
   something nobody can name afterwards. `ilk preview url --upload` refuses outside CI
   for this reason. Never run `wrangler deploy`, `wrangler versions deploy` or
   `wrangler rollback` — each one changes production — and leave `wrangler preview
   delete` to a person.

2. **Get the URL.**

   ```sh
   ilk preview url              # the current branch
   ilk preview url feat/x --json
   ```

   This works the URL out locally and calls nothing. If it says
   `preview.workers_subdomain` is not set, ask for the account's workers.dev
   subdomain (it is not a secret) rather than guessing it. For a branch whose name is
   not already lowercase letters, digits and dashes, the build's comment on the pull
   request has the URL Cloudflare actually gave it; use that one.

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

4. **Read, never write.** A Preview's Durable Objects are its own, but a store the
   `previews` block binds may still be production's, and an uploaded version runs with
   production's bindings outright (`preview.separate-data` says which). Treat a write
   in a preview as a write to production. `ilk preview open`
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
| Error 1101 | The Preview ran and threw. Most often a binding production has that the `previews` block leaves out (`preview.previews-complete`); a Preview inherits none. |
| Error 1042 | No version preview exists at that address. A Worker that implements a Durable Object gets no Version URL; it needs Workers Previews (`preview.workers-previews`). |
| No URL on the pull request | Workers Builds made no Preview: Enable Preview Builds is off, the Preview command is not `npx wrangler preview`, or Wrangler is older than 4.135. A person fixes the first two in the dashboard. |
| The build failed | Read its log. For uploaded versions, a change that adds, renames or deletes a Durable Object class cannot be uploaded at all; it ships with the production deploy. Say so rather than retrying. |

Do not turn on `workers_dev`, remove the Access check, or loosen a policy to get a
preview to answer. Each of those is the thing the checks exist to catch.

## Teardown

Nothing for you to tear down. A Preview is not deployed, so it serves nothing on
production's routes; pushing the branch again updates the same Preview, and Cloudflare
drops the least recently deployed Preview once a Worker has 100 (500 on paid plans).
Deleting one sooner (`wrangler preview delete`) also deletes its Durable Object data,
so leave it to a person.
