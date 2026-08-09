---
id: spec-retry-policy
title: One retry policy, in the gateway
status: proposed
updated: 2026-08-08
---

# One retry policy, in the gateway

## Acceptance criteria

- A 502 from any upstream is retried three times with jitter.
- Nothing a user can see changes.

No `ui:` key, because nothing anybody looks at moves.
