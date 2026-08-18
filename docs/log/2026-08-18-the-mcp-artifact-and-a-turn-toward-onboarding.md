---
date: 2026-08-18
title: The mcp artifact, and a turn toward onboarding
---

# The mcp artifact, and a turn toward onboarding

Two things happened: the `mcp:` neutral artifact — item 2 in the layer queue —
landed, and the rest of the queue was deferred in favour of documentation and
onboarding, on a decision from Jason.

## The artifact

A layer now declares an MCP server once, under `mcp:` in the manifest, and each
configured target projects it: `.mcp.json` for Claude Code, `.cursor/mcp.json`
for Cursor, both through one merge over the shared `mcpServers` shape. Both
files are co-owned — joined and vacated like `.claude/settings.json`, never
overwritten, never deleted while the user has entries of their own in them.

The design decision worth recording is the indirection. Every entry ilk writes
says `ilk mcp run <name>`; the real command stays in the manifest and is
resolved when the agent starts the server. That one choice bought three things
at once:

- **Recognisable ownership without marker keys.** The settings.json precedent —
  identify ilk's entries by their command string, in a schema ilk does not own —
  carries over unchanged, which is what lets a vacate strip exactly ilk's
  entries without knowing which layer wrote them.
- **Stable agent config.** Changing a server's `command:`, `args:` or `env:`
  never rewrites `.mcp.json`, for the same reason adding a hook never rewrites
  settings.json.
- **The credential story, applied.** `requires_env:` names the variables; `ilk
  mcp run` tests presence without reading values and refuses to start with the
  missing name in the message, instead of the agent reporting an opaque
  connection failure. Nothing secret is ever written to a committed file.

Server names are repository-wide: two layers declaring the same name refuse to
plan. `ilk mcp list` shows what is declared, by which layer, and which servers
are currently missing a credential. `ilk info` and the `ilk layer new` scaffold
learned the new block; the toolkit skill teaches it, so toolkit went to 0.3.0.

This closes the last artifact type where a layer would otherwise have shipped
one agent's literal file, and unblocks `mcp-servers` and the version of
`codegraph` worth having.

## The turn

With the queue's two core items done, the next investment is not more layers.
The M2 acceptance criterion — an outsider publishes a layer and adopting it
needs no core change — has never been exercised, and more first-party layers
cannot exercise it. What can is a stranger getting from zero to a working
repository, and every path a stranger would take today runs through documents
written for people who were already here.

So the remaining queue items are deferred, order preserved, and
[PLAN-docs-onboarding](../plans/PLAN-docs-onboarding.md) is the active plan:
first a one-line install (there are still no tagged releases), then the first
fifteen minutes, then the layer author's path, then a docs site pulled forward
from M4.
