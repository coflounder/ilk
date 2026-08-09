---
id: q-retry-ownership
title: Should retries live in the gateway or in each client?
status: open
blocking: true
kind: question
asked: 2026-08-08
---

# Should retries live in the gateway or in each client?

## Why this needs a person

It is expensive to reverse once four services depend on the behaviour.

## What I would do without an answer

Put them in the gateway, because it is the only place that can see the whole failure.

## What is blocked

The retry work itself. Everything else in the milestone can go ahead.

## Answer
