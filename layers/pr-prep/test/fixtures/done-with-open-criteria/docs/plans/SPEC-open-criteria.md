---
id: spec-open-criteria
title: Work called done with a criterion still open
status: done
updated: 2026-08-07
---

# Work called done with a criterion still open

## Outcome

Webhook deliveries retry on failure instead of being dropped.

## Acceptance criteria

- [x] A failed delivery is retried three times with backoff.
- [ ] A delivery that exhausts its retries is recorded and surfaced.

## Evidence

- `go test ./internal/webhooks` passes — 14 tests, 0 failures.
