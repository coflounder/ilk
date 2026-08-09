---
id: q-retry-ownership
title: Should retries live in the gateway?
status: open
blocking: true
kind: decision
asked: 2026-08-08
options:
  - id: gateway
    label: One retry policy, in the gateway
    consequence: One place to tune, and every caller inherits the gateway's outages as well as its policy.
recommended: gateway
---

# Should retries live in the gateway?

## What I would do without an answer

The gateway, obviously, since it is the only thing on the list.
