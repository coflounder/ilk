---
date: 2026-11-19
title: The preview layer moves to Workers Previews
---

# The preview layer moves to Workers Previews

`layers/preview` 0.3.0 makes Workers Previews (`wrangler preview`, configured by a
`previews` block) the way a pull request is previewed. Until now the layer asked for
uploaded versions with a preview alias and treated Workers Previews as an exception
for Workers that implement a Durable Object.

Why: Cloudflare's Workers Previews docs (Compare workflows, November 18) say not to use
Version URLs for branch or pull request testing and to switch aliased Version URLs to
Previews. A version runs with production's bindings; a Preview gets only what the
`previews` block gives it, including its own Durable Object namespace. Healthy Bitz
moved to Workers Previews on November 18, after version uploads gave it no preview URL.

What changed:

- `preview.workers-previews` replaces `preview.no-durable-objects`, and fails without a
  `previews` block. A repository that keeps versions on purpose skips it.
- `preview.previews-complete` is new: every binding and var production has must be in
  the `previews` block, because a Preview inherits none (a missing Durable Object
  binding answers error 1101 — Healthy Bitz added its own test for this).
- `preview.preview-audience` also looks in `previews.vars`.
- `ilk preview url --upload` runs `wrangler preview --name <name>`, and falls back to
  `versions upload --preview-alias` with a warning only when there is no `previews`
  block.
- The skill, the instructions and the README describe Previews, error 1101, and
  `wrangler preview delete` for teardown.

Still unknown: how Cloudflare turns a branch name with characters outside lowercase
letters, digits and dashes into a Preview name. `ilk preview url` assumes the alias
rules, and says so.
