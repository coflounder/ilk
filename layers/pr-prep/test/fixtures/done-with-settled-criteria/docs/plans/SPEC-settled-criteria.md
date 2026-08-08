---
id: spec-settled-criteria
title: Work called done with every criterion ticked
status: done
updated: 2026-08-07
---

# Work called done with every criterion ticked

## Outcome

Webhook deliveries retry on failure instead of being dropped.

## Acceptance criteria

- [x] A failed delivery is retried three times with backoff.
- [x] A delivery that exhausts its retries is recorded and surfaced.

## Evidence

- `go test ./internal/webhooks` passes — 14 tests, 0 failures.
