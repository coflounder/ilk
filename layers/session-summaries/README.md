# Session summaries

Shared context for independently launched coding agents. Each agent writes a concise,
cumulative summary of its own conversation. A project MCP makes those publications
searchable across harnesses and linked Git worktrees. Agents keep their own tools,
processes and transcripts, and continue in the task's existing working directory.

## Install

Requires Python 3.10+, Git and [uv](https://docs.astral.sh/uv/getting-started/installation/).
Use an ilk build containing the `turn-end` event introduced with this layer.

```sh
ilk add gh:coflounder/ilk/layers/session-summaries --allow-exec
ilk agents add codex
ilk agents add pi
ilk mcp list
ilk check --only sessions.runtime
```

For an unpublished local checkout, use its absolute layer directory instead of the
`gh:` source. Enable whichever targets you use with `ilk agents add <target>`.

| Harness | Generated integration | Checkpoints |
|---|---|---|
| Claude Code | `.claude/settings.json` hooks and `.mcp.json` | Native SessionStart and Stop |
| Codex | `.codex/hooks.json` and a fenced MCP section in `.codex/config.toml` | Native SessionStart and Stop |
| Pi | `.pi/extensions/ilk/` with a native extension and MCP client | Session startup and agent-settled events |
| Cursor | `.cursor/mcp.json` | Explicit publication |

Codex's user configuration and comments are preserved. A user-owned server with the
same name is a conflict, not permission to overwrite it. Codex requires trusting the
project and reviewing generated hooks with `/hooks`; ilk does not change trust or
approval settings. Hook support must be enabled in the harness. See the
[official hooks guide](https://developers.openai.com/codex/hooks).

Pi loads the project extension after project trust. It registers the MCP server's
actual tool schemas as native Pi tools, with names such as
`ilk_session_summaries__session_summary_save`. The extension keeps one MCP connection
per server for the session, forwards cancellation, and closes connections on reload
or shutdown. It uses the official Python MCP client; no separate Pi MCP package or
npm install is needed by adopters. Pi already discovers `.agents/skills` directly.

The adapters are tested with Codex 0.153.4 and Pi 0.85.1 (Node 22.19+). The MCP server
and Pi client use the official Python SDK, pinned in their scripts; uv prepares their
environments on first connection. Hooks and CLI commands use Python's standard
library. Restart a running harness after adoption, or use Pi's `/reload`.

Another MCP-capable harness can register `ilk mcp run session-summaries` as a stdio
server, with the task's worktree as its working directory. CLI commands remain
available to every harness. All agents are launched directly; ilk starts no agents.

## Publication lifecycle

On Claude Code, Codex and Pi, `session-start` shows recent summary metadata. The
agent searches and reads relevant summaries rather than loading the entire project
history into context.
The layer maps `turn-end` to native Stop events on Claude/Codex, and Pi's
`agent_settled` event after automatic retries and compaction have finished:

1. The hook stores a checkpoint request and asks the active agent to write its summary.
2. The agent calls `session_summary_save` with the checkpoint ID and expected revision.
3. The tool validates and stages the draft, without replacing the published summary.
4. At the next stop, the hook atomically publishes the draft.

There is at most one requested continuation. If the agent stops again without a draft,
the checkpoint is marked `missed`; the previous publication survives. Pending, staged,
and missed checkpoints are visible through list/read. A later normal turn supersedes
an interrupted request, so an old draft cannot be mistaken for a new checkpoint.
Interrupted or failed runs do not force a summary continuation. This is checkpointed
context, not a crash-proof transcript backup. Pi uses a custom context message for
its continuation, so the summary request is not impersonated user input.

The active agent creates the summary; no other model or harness is launched. Its
quality still depends on that agent. The hook checks the publication shape and
lifecycle, not whether every statement is true.

Harnesses without a native adapter publish explicitly through MCP or CLI before
handoff or compaction. The `share-session-context` skill describes both workflows.

## Tools and commands

| MCP tool | CLI | Use |
|---|---|---|
| `session_summaries_list` | `ilk session-summaries list --query TEXT --limit 20` | Literal text search over published summaries; discover unfinished checkpoints |
| `session_summary_read` | `ilk session-summaries read HARNESS SESSION` | Full publication, revision, provenance and checkpoint status |
| `session_summary_save` | `ilk session-summaries save FILE` | Stage a requested checkpoint or publish explicitly |

All CLI results are JSON. Save accepts `-` for stdin. A minimal explicit publication:

```json
{
  "harness": "pi",
  "session_id": "your-stable-session-id",
  "expected_revision": 0,
  "source_ref": "native transcript path or checkpoint reference",
  "summary": {
    "task": "Fix login redirect",
    "objective": "Return users to the page they requested",
    "constraints": "Keep the current authentication provider",
    "decisions": "Store only a validated relative return path",
    "completed": "Identified the redirect handling code; no changes yet",
    "remaining": "Implement validation and test the redirect",
    "verification": "Not run; investigation only",
    "questions": "None",
    "next_actions": "Add a failing test for the return path"
  }
}
```

Read first and use the returned revision for subsequent saves. A stale revision is
rejected; reconcile with the current summary instead of overwriting it. Hook requests
also require their checkpoint ID. All nine summary fields are required, nonempty
strings, and the total summary is limited to 32 KiB. Use explicit `None`/`Not run`
where appropriate.

## Storage and evidence

The store is `<git-common-dir>/ilk/session-summaries/summaries.sqlite3`, resolved by
Git. Linked worktrees share it; separate clones do not. SQLite transactions serialize
publication and reject stale writers. Sessions are keyed by both harness and native
session ID. The local store is outside tracked files and is preserved when the layer
is removed. No server daemon, external database, network listener, or history index
is needed. Git push does not synchronize this store to other machines.

The server stamps the actual working directory, branch, HEAD and dirty status. This
is not a fingerprint of uncommitted content, nor proof that tests ran against it.
Verification prose must state actual commands and outcomes. Native transcript paths
remain references only: the server does not read or import them. A checkpoint records
its capture time and the digest of the Stop event's final message, which need not yet
be present in the native transcript. Summaries can be stale, incomplete or wrong;
they do not authorize actions or override current user/project instructions.

The source boundary ends when the hook requests the summary. After staging, stop
without doing more project work. Accepted architectural decisions still belong in
the project's maintained record. Session summaries complement that record.

## Verify

```sh
ilk layer validate layers/session-summaries
ilk layer test layers/session-summaries
sh layers/session-summaries/test/run.sh
```

Tests exercise real linked worktrees, separate project isolation, revision conflicts,
concurrent publication, interrupted/missed checkpoints, CLI publication and real MCP
stdio exchanges using independently started server processes. Harness tests load the
real Pi extension SDK, run the registered tools and lifecycle handlers, exercise
reload/shutdown, and verify generated MCP configuration with the actual Codex CLI.
Native Stop payload fixtures cover all three harness identities. They do not launch
a paid model session or prove that an agent will write an accurate summary.
The test runner installs pinned harness packages into a disposable directory.

Hook contract: [Claude Code hooks](https://code.claude.com/docs/en/hooks#stop).
MCP implementation: [official Python SDK](https://github.com/modelcontextprotocol/python-sdk).
