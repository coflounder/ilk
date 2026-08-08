---
id: spec-in-progress
title: Work still in progress, with criteria still open
status: active
updated: 2026-08-07
owner: the-webhooks-team
---

# Work still in progress, with criteria still open

## Outcome

Webhook deliveries retry on failure instead of being dropped.

## Acceptance criteria

- [ ] A failed delivery is retried three times with backoff.
- [ ] A delivery that exhausts its retries is recorded and surfaced.
