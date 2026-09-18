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
ilk mcp list
ilk check --only sessions.runtime
```

For an unpublished local checkout, use its absolute layer directory instead of the
`gh:` source. MCP registration is generated for the configured Claude Code and Cursor
targets. For another MCP-capable harness, register this stdio command in that harness's
own configuration, with the task's repository/worktree as its working directory:

```sh
ilk mcp run session-summaries
```

Pi needs an MCP extension; this layer does not install one. Targets without MCP can
use the equivalent `ilk session-summaries` CLI commands. The server uses the official
MCP Python SDK, pinned in the script; uv installs it on first connection. Hooks and
CLI commands use only Python's standard library, so publishing does not need an
SDK download. Restart an existing harness after adopting the layer.

## Publication lifecycle

On Claude Code, `session-start` shows recent summary metadata. The agent searches and
reads relevant summaries rather than loading the entire project history into context.
The layer maps `turn-end` to Claude's native `Stop` event:

1. The hook stores a checkpoint request and asks the active agent to write its summary.
2. The agent calls `session_summary_save` with the checkpoint ID and expected revision.
3. The tool validates and stages the draft, without replacing the published summary.
4. At the next stop, the hook atomically publishes the draft.

There is at most one requested continuation. If the agent stops again without a draft,
the checkpoint is marked `missed`; the previous publication survives. Pending, staged,
and missed checkpoints are visible through list/read. A later normal turn supersedes
an interrupted request, so an old draft cannot be mistaken for a new checkpoint.
Stop hooks do not run on user interruption or API failure. This is checkpointed
context, not a crash-proof transcript backup.

The active agent creates the summary; no other model or harness is launched. Its
quality still depends on that agent. The hook checks the publication shape and
lifecycle, not whether every statement is true.

Other harnesses publish explicitly through MCP or CLI before handoff or compaction.
Automatic end-of-turn integration is currently Claude-only. The `share-session-context`
skill and generated instructions describe both workflows.

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
stdio exchanges using two independently started server processes. They do not launch
a paid model session or prove that an agent will write an accurate summary.

Hook contract: [Claude Code hooks](https://code.claude.com/docs/en/hooks#stop).
MCP implementation: [official Python SDK](https://github.com/modelcontextprotocol/python-sdk).
