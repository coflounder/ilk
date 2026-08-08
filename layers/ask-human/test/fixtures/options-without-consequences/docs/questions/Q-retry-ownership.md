---
id: q-retry-ownership
title: Should retries live in the gateway or in each client?
status: open
blocking: true
kind: decision
asked: 2026-08-08
options:
  - id: gateway
    label: In the gateway
    consequence:
  - id: client
    label: In each client
    consequence:
recommended: gateway
---

# Should retries live in the gateway or in each client?

## What I would do without an answer

The gateway. Two labels and no stated difference between them, so the reader is being
asked to supply the reasoning as well as the decision.
