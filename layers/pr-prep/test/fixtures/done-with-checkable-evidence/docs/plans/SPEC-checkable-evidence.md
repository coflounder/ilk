---
id: spec-checkable-evidence
title: Finished work whose evidence somebody else can open
status: done
updated: 2026-08-07
---

# Finished work whose evidence somebody else can open

## Outcome

Webhook deliveries retry on failure instead of being dropped.

## Acceptance criteria

- [x] A failed delivery is retried three times with backoff.

## Evidence

- `go test ./internal/webhooks` — 14 tests, 0 failures.
- Shipped in [#412](https://example.invalid/pull/412), green on CI.
