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
recommended: gateway
---

# Should retries live in the gateway or in each client?

## Why this needs a person

Expensive to reverse once four services depend on the behaviour, and the two options
put the cost on different teams.

## What I would do without an answer

The gateway. It is the only place that can see the whole failure, and the drift the
other option invites is the kind nobody notices until an incident.

## What is blocked

The retry work. Everything else in the milestone can go ahead.

## Answer
