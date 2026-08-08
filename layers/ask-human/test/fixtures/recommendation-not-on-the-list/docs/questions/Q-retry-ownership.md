---
id: q-retry-ownership
title: Should retries live in the gateway or in each client?
status: open
blocking: true
kind: decision
asked: 2026-08-08
options:
  - id: gateway
    label: One retry policy, in the gateway
    consequence: One place to tune, and every caller inherits the gateway's outages as well as its policy.
  - id: client
    label: Each client owns its retries
    consequence: Callers tune for their own tolerance; four codebases now hold a retry policy and three of them will drift.
recommended: sidecar
---

# Should retries live in the gateway or in each client?

## What I would do without an answer

A sidecar — which is not one of the options, so whoever answers has to work out
whether the list or the recommendation is the mistake.
