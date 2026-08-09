---
id: q-retry-ownership
title: Should retries live in the gateway or in each client?
status: answered
blocking: true
kind: question
asked: 2026-08-08
---

# Should retries live in the gateway or in each client?

## Why this needs a person

It is expensive to reverse once four services depend on the behaviour.

## Answer

- The gateway. One policy, and the callers stop caring. Decided by the platform owner
  on 2026-08-08; promoted to `docs/reference/DEC-retries-in-the-gateway.md`.
