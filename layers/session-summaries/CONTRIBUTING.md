# Contributing session summaries

Include a reproducible hook payload or a synthetic MCP exchange. Never attach real
private transcripts. Explain what a receiving agent failed to recover, and whether
the failure was in the publication lifecycle, summary shape, or harness integration.

Run `ilk layer validate layers/session-summaries`, `ilk layer test layers/session-summaries`,
and `sh layers/session-summaries/test/run.sh`. Protocol tests use the pinned official
MCP Python SDK. Keep direct harness invocation and one project store across linked
worktrees; changes must not introduce an agent orchestrator or a transcript index.
